package main

import (
	"context"
	"encoding/json"
	"fmt"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/idoubi/goz"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/qiniu/api.v7/v7/auth/qbox"
	"github.com/qiniu/api.v7/v7/storage"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"
	"webimage/asset"
)
const (
	upload_path string = "/expo/"
)
var (
	DBM *gorm.DB
	DBS *gorm.DB
	GVA_CONFIG *Server
	GVA_LOG    *zap.Logger
)

type (
	Server struct {
		Mysql Mysql `mapstructure:"mysql" json:"mysql" yaml:"mysql"`
		Qiniu Qiniu `mapstructure:"qiniu" json:"qiniu" yaml:"qiniu"`
		BaiduAi BaiduAi `mapstructure:"baidu-ai" json:"baiduAi" yaml:"baidu-ai"`
		System  System  `mapstructure:"system" json:"system" yaml:"system"`
	}
	System struct {
		Env           string `mapstructure:"env" json:"env" yaml:"env"`
		Addr          int `mapstructure:"addr" json:"addr" yaml:"addr"`
	}
	Mysql struct {
		Host         string `mapstructure:"host" json:"Host" yaml:"host"`
		Config       string `mapstructure:"config" json:"config" yaml:"config"`
		Dbname       string `mapstructure:"db-name" json:"dbname" yaml:"db-name"`
		Username     string `mapstructure:"username" json:"username" yaml:"username"`
		Password     string `mapstructure:"password" json:"password" yaml:"password"`
		Port         int   	`mapstructure:"port" json:"password" yaml:"port"`
		MaxIdleConns int    `mapstructure:"max-idle-conns" json:"maxIdleConns" yaml:"max-idle-conns"`
		MaxOpenConns int    `mapstructure:"max-open-conns" json:"maxOpenConns" yaml:"max-open-conns"`
		LogMode      bool   `mapstructure:"log-mode" json:"logMode" yaml:"log-mode"`
	}
	Qiniu struct {
		Zone          string `mapstructure:"zone" json:"zone" yaml:"zone"`
		Bucket        string `mapstructure:"bucket" json:"bucket" yaml:"bucket"`
		ImgPath       string `mapstructure:"img-path" json:"imgPath" yaml:"img-path"`
		UseHTTPS      bool   `mapstructure:"use-https" json:"useHttps" yaml:"use-https"`
		AccessKey     string `mapstructure:"access-key" json:"accessKey" yaml:"access-key"`
		SecretKey     string `mapstructure:"secret-key" json:"secretKey" yaml:"secret-key"`
		UseCdnDomains bool   `mapstructure:"use-cdn-domains" json:"useCdnDomains" yaml:"use-cdn-domains"`
	}
	BaiduAi struct {
		AppKey			string `mapstructure:"app-key" json:"appKey" yaml:"app-key"`
		AppSecretKey	string `mapstructure:"app-secret-key" json:"appSecretKey" yaml:"app-secret-key"`
	}
	returnData struct {
		Code 	int `json:"code"`
		Msg 	string `json:"msg"`
		Result 	interface{} `json:"result"`
	}
)

func init() {
	path, err := os.Getwd()
	if err != nil {
		GVA_LOG.Error("Yaml configuration file loading failed:",zap.Any("err",err))
	}
	config := viper.New()
	config.AddConfigPath(path)
	config.SetConfigName("./config")
	config.SetConfigType("yaml")
	if err := config.ReadInConfig(); err != nil {
		GVA_LOG.Error("Read configuration error:",zap.Any("err",err))
	}
	if err = config.Unmarshal(&GVA_CONFIG); err != nil {
		GVA_LOG.Error("Mapping configuration file failed:",zap.Any("err",err))
	}

	DBS = DbConn(GVA_CONFIG.Mysql.Username,GVA_CONFIG.Mysql.Password,GVA_CONFIG.Mysql.Host,GVA_CONFIG.Mysql.Dbname,GVA_CONFIG.Mysql.Port,GVA_CONFIG.Mysql.Config)
	DBS.DB().SetMaxIdleConns(GVA_CONFIG.Mysql.MaxIdleConns)                   // Maximum number of idle connections
	DBS.DB().SetMaxOpenConns(GVA_CONFIG.Mysql.MaxOpenConns)                   // maximum connection
	DBS.DB().SetConnMaxLifetime(time.Second * 300)                            // Set connection idle timeout
	DBS.LogMode(GVA_CONFIG.Mysql.LogMode)
}

func DbConn(MyUser, Password, Host, Db string, Port int,Config string) *gorm.DB {
	connArgs := fmt.Sprintf("%s:%s@(%s:%d)/%s?%s",  MyUser,Password, Host, Port, Db, Config )
	db, err := gorm.Open("mysql", connArgs)
	if err != nil {
		GVA_LOG.Error("MySQL link failed:",zap.Any("err",err))
	}
	db.SingularTable(true)
	return db
}
// load webimageInfo
func webimageInfo(w http.ResponseWriter,r *http.Request) {
	bytes, err := asset.Asset("public/view/webimage.html")
	if err != nil {
		GVA_LOG.Error("Image settings page loading failed:",zap.Any("err",err))
	}
	t, err := template.New("index").Delims("<<", ">>").Parse(string(bytes))

	t = template.Must(t, err)
	if err != nil {
		GVA_LOG.Error("Failed to load page template:",zap.Any("err",err))
	}
	var fileList = make(map[string]interface{})

	t.Execute(w, fileList)
}

// save card info
func saveCard(w http.ResponseWriter, r *http.Request)  {
	cardDiscountCode :=r.PostFormValue("gift_code_num")
	p := &returnData{}
	if len(cardDiscountCode) < 5 {
		p.Code = 400
		p.Msg = "礼品优惠码不能为空~"
		data, _ := json.Marshal(p)
		fmt.Fprintln(w,string(data))
		return
	}
	cardNumber := r.PostFormValue("gift_card_num")
	if len(cardNumber) < 5 {
		p.Code = 400
		p.Msg = "礼品卡编号不能为空~"
		data, _ := json.Marshal(p)
		fmt.Fprintln(w,string(data))
		return
	}
	cardMount :=r.PostFormValue("gift_amount")
	cardMountF, _ := strconv.ParseFloat(cardMount, 64)
	if cardMountF == 0 {
		p.Code = 400
		p.Msg = "礼品卡金额不能为空~"
		data, _ := json.Marshal(p)
		fmt.Fprintln(w,string(data))
		return
	}
	// 插入数据组装
	type Giftcards struct {
		CardDiscountCode  	string `json:"card_discount_code"`
		CardNumber    		string  `json:"card_number"`
		CardMount			float64	`json:"card_mount"`
		CardBalance			float64 `json:"card_balance"`
		CreatedTime			int64
	}
	var createdTime = time.Now().UTC().Unix()
	giftcardsInfo := &Giftcards{
		CardDiscountCode: cardDiscountCode,
		CardNumber:cardNumber,
		CardMount:cardMountF,
		CardBalance:cardMountF,
		CreatedTime: createdTime,
	}
	// insert database
	err := DBS.Table("apple_giftcards").Create(&giftcardsInfo).Error
	if err !=nil {
		p.Code = 400
		p.Msg = "卡号已提交，无需要重复提交~"
		data, _ := json.Marshal(p)
		fmt.Fprintln(w,string(data))
		return
	}
	// success
	returnMap := make(map[string]interface{})
	returnMap["card_info"] =giftcardsInfo
	p.Result = returnMap
	data, _ := json.Marshal(p)
	fmt.Fprintln(w,string(data))

}

// ajax upload image
func uploadImage(w http.ResponseWriter, r *http.Request)  {
	file, head, err := r.FormFile("file")
	if err != nil {
		GVA_LOG.Error("Failed to upload image to form:",zap.Any("err",err))
	}
	defer file.Close()
	// create Dir
	pwd, _ := os.Getwd()
	// If the folder exists, an error will be returned. You can use the`_ `Throw it out
	err = os.Mkdir(pwd+upload_path, os.ModePerm)
	if err != nil {
		fmt.Println("dir is create Error")
	}
	fW, err := os.Create(pwd + upload_path + head.Filename)
	if err != nil {
		GVA_LOG.Error("File create failed:",zap.Any("err",err))
	}
	fmt.Println(*fW)
	defer fW.Close()
	// Copy the file and save it locally
	_, err = io.Copy(fW, file)
	if err != nil {
		GVA_LOG.Error("File save failed:",zap.Any("err",err))
	}
	imageUrl :=upload_qiniu(pwd + upload_path + head.Filename)
	url :="https://aip.baidubce.com/rest/2.0/ocr/v1/webimage?access_token="+getBaiduAccessToken()
	cli := goz.NewClient()

	resp, err := cli.Post(url, goz.Options{
		Headers: map[string]interface{}{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		FormParams: map[string]interface{}{
			"url": imageUrl,
		},
	})
	if err != nil {
		GVA_LOG.Error("Failed to request Baidu AI image recognition interface",zap.Any("err",err))
	}
	body,_ := resp.GetBody()
	type BdImgResponeData struct {
		LogId uint `json:"log_id"`
		WordsResultNum  uint `json:"words_result_num"`
		WordsResult[]  struct{
			Words string `json:"words"`
		} `json:"words_result"`
	}
	imageInfo := BdImgResponeData{}
	json.Unmarshal([]byte(body),&imageInfo)

	p := &returnData{}
	p.Code = 200
	p.Msg = "success~"
	returnMap := make(map[string]interface{})
	returnMap["words_result"] =imageInfo.WordsResult[0].Words
	p.Result = returnMap
	data, _ := json.Marshal(p)
	fmt.Fprintln(w,string(data))
}
// upload image to qiniu
func upload_qiniu(filePath string) (imageUrl string){
	key := "webimage"+ GetDateYMDHis() +".png"
	putPolicy := storage.PutPolicy{
		Scope: GVA_CONFIG.Qiniu.Bucket,
	}
	mac := qbox.NewMac(GVA_CONFIG.Qiniu.AccessKey, GVA_CONFIG.Qiniu.SecretKey)
	upToken := putPolicy.UploadToken(mac)
	cfg := storage.Config{}
	cfg.Zone = &storage.ZoneHuanan
	cfg.UseHTTPS = false
	cfg.UseCdnDomains = false
	//Build upload form object
	formUploader := storage.NewFormUploader(&cfg)
	ret := storage.PutRet{}
	// Optional
	putExtra := storage.PutExtra{
		Params: map[string]string{
			"x:name": "github logo",
		},
	}
	err := formUploader.PutFile(context.Background(), &ret, upToken, key, filePath, &putExtra)
	if err != nil {
		GVA_LOG.Error("Failed to upload picture to 7niu",zap.Any("err",err))
	}
	fmt.Println(ret.Key)
	imageUrl = GVA_CONFIG.Qiniu.ImgPath + ret.Key
	return
}
// date YYMMDDHHiiss
func GetDateYMDHis()  (date_time string) {
	now := time.Now().UTC()
	month := time.Unix(1557042972, 0).Format("1")
	year := now.Format("2006")
	month = now.Format("01")
	day := now.Format("02")
	hour := strconv.Itoa(now.Hour())
	minute := strconv.Itoa(now.Minute())
	second := strconv.Itoa(now.Second())

	date_time = year + month + day + hour + minute + second
	return
}

// Get Baidu AccessToken
func getBaiduAccessToken() (accessToken string) {
	url :="https://aip.baidubce.com/oauth/2.0/token"
	postParam := make(map[string]interface{})
	postParam["grant_type"] = "client_credentials"
	postParam["client_id"] = GVA_CONFIG.BaiduAi.AppKey
	postParam["client_secret"] = GVA_CONFIG.BaiduAi.AppSecretKey
	cli := goz.NewClient()
	resp, err := cli.Get(url, goz.Options{
		Query: postParam,
	})
	if err != nil {
		// record err logs
	}
	body,_ := resp.GetBody()
	type BaiduRespnseData struct {
		ExpiresIn  		uint  	`json:"expires_in"`
		AccessToken		string	`json:"access_token"`
	}
	returnData :=&BaiduRespnseData{}
	json.Unmarshal([]byte(body),&returnData)
	accessToken = returnData.AccessToken
	return
}

func main()  {
	fs := assetfs.AssetFS{
		Asset:     asset.Asset,
		AssetDir:  asset.AssetDir,
		Prefix:"public",
	}

	http.Handle("/", http.FileServer(&fs))
	http.HandleFunc("/webimage", webimageInfo)	// webimage
	http.HandleFunc("/webimage/uploadImage",uploadImage) // upload image~
	http.HandleFunc("/webimage/saveCard",saveCard) // save card~

	http.ListenAndServe(":"+strconv.Itoa(GVA_CONFIG.System.Addr), nil)

}