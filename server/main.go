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

var (
	DBM *gorm.DB
	DBS *gorm.DB
	GVA_CONFIG *Server
	GVA_LOG    *zap.Logger
)

type Server struct {
	// gorm
	Mysql Mysql `mapstructure:"mysql" json:"mysql" yaml:"mysql"`
	Qiniu Qiniu `mapstructure:"qiniu" json:"qiniu" yaml:"qiniu"`
	BaiduAi BaiduAi `mapstructure:"baidu-ai" json:"baiduAi" yaml:"baidu-ai"`
	System  System  `mapstructure:"system" json:"system" yaml:"system"`
}

type System struct {
	Env           string `mapstructure:"env" json:"env" yaml:"env"`
	Addr          string    `mapstructure:"addr" json:"addr" yaml:"addr"`
}

type Mysql struct {
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

type Qiniu struct {
	Zone          string `mapstructure:"zone" json:"zone" yaml:"zone"`
	Bucket        string `mapstructure:"bucket" json:"bucket" yaml:"bucket"`
	ImgPath       string `mapstructure:"img-path" json:"imgPath" yaml:"img-path"`
	UseHTTPS      bool   `mapstructure:"use-https" json:"useHttps" yaml:"use-https"`
	AccessKey     string `mapstructure:"access-key" json:"accessKey" yaml:"access-key"`
	SecretKey     string `mapstructure:"secret-key" json:"secretKey" yaml:"secret-key"`
	UseCdnDomains bool   `mapstructure:"use-cdn-domains" json:"useCdnDomains" yaml:"use-cdn-domains"`
}

type BaiduAi struct {
	AppKey		string `mapstructure:"app-key" json:"appKey" yaml:"app-key"`
	AppSecretKey		string `mapstructure:"app-secret-key" json:"appSecretKey" yaml:"app-secret-key"`
}

// return data
type returnData struct {
	Code int `json:"code"`
	Msg string `json:"msg"`
	Result interface{} `json:"result"`
}

const (
	//本地保存的文件夹名称
	upload_path string = "/expocraftsmen/"
)

func init() {
	path, err := os.Getwd()
	if err != nil {
		GVA_LOG.Error("文件加载~",zap.Any("err",err))
	}
	config := viper.New()
	config.AddConfigPath(path)
	config.SetConfigName("./config")
	config.SetConfigType("yaml")
	if err := config.ReadInConfig(); err != nil {
		GVA_LOG.Error("读取配置",zap.Any("err",err))
	}
	if err = config.Unmarshal(&GVA_CONFIG); err != nil {
		GVA_LOG.Error("隐射配置",zap.Any("err",err))
	}

	DBS = DbConn(GVA_CONFIG.Mysql.Username,GVA_CONFIG.Mysql.Password,GVA_CONFIG.Mysql.Host,GVA_CONFIG.Mysql.Dbname,GVA_CONFIG.Mysql.Port)
	DBS.DB().SetMaxIdleConns(GVA_CONFIG.Mysql.MaxIdleConns)                   //最大空闲连接数
	DBS.DB().SetMaxOpenConns(GVA_CONFIG.Mysql.MaxOpenConns)                   //最大连接数
	DBS.DB().SetConnMaxLifetime(time.Second * 300)                            //设置连接空闲超时
	DBS.LogMode(GVA_CONFIG.Mysql.LogMode)
	// defer DBS.Close()
}

func DbConn(MyUser, Password, Host, Db string, Port int) *gorm.DB {
	connArgs := fmt.Sprintf("%s:%s@(%s:%d)/%s?charset=utf8&parseTime=True&loc=Local",  MyUser,Password, Host, Port, Db )
	db, err := gorm.Open("mysql", connArgs)
	if err != nil {
		GVA_LOG.Error("Mysql链接错误:",zap.Any("err",err))
	}
	db.SingularTable(true)
	return db
}
// load webimageInfo
func webimageInfo(w http.ResponseWriter,r *http.Request) {
	bytes, err := asset.Asset("public/view/webimage.html")  // 根据地址获取对应内容
	if err != nil {
		GVA_LOG.Error("页面跑丢了:",zap.Any("err",err))
	}
	t, err := template.New("index").Delims("<<", ">>").Parse(string(bytes))      // 比如用于模板处理

	t = template.Must(t, err)
	if err != nil {
		GVA_LOG.Error("百度模板加载:",zap.Any("err",err))
	}
	var fileList = make(map[string]interface{})

	t.Execute(w, fileList)
}

// save card info
func saveCard(w http.ResponseWriter, r *http.Request)  {
	cardDiscountCode :=r.PostFormValue("gift_code_num")
	fmt.Println("cardDiscountCode",len(cardDiscountCode))
	p := &returnData{}
	if len(cardDiscountCode) < 5 {
		p.Code = 400
		p.Msg = "礼品优惠码不能为空~"
		data, _ := json.Marshal(p)
		fmt.Fprintln(w,string(data))
		return
	}
	cardNumber := r.PostFormValue("gift_card_num")
	fmt.Println("cardNumber",len(cardNumber))
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
		CardBalance			float64  `json:"card_balance"`
		CreatedTime			interface{}
	}
	var createdTime = time.Now().UTC().Unix()
	giftcardsInfo := &Giftcards{
		CardDiscountCode: cardDiscountCode,
		CardNumber:cardNumber,
		CardMount:cardMountF,
		CardBalance:cardMountF,
		CreatedTime: createdTime,
	}
	//写入数据库
	err := DBS.Table("apple_giftcards").Create(&giftcardsInfo).Error
	if err !=nil {
		p.Code = 400
		p.Msg = "卡号已提交，无需要重复提交~"
		data, _ := json.Marshal(p)
		fmt.Fprintln(w,string(data))
		return
	}

	returnMap := make(map[string]interface{})
	returnMap["card_info"] =giftcardsInfo
	p.Result = returnMap
	data, _ := json.Marshal(p)
	fmt.Fprintln(w,string(data))

}

//ajax 上传图片~
func uploadImage(w http.ResponseWriter, r *http.Request)  {
	file, head, err := r.FormFile("file")
	if err != nil {
		GVA_LOG.Error("上传图片:",zap.Any("err",err))
	}
	defer file.Close()
	//创建文件夹
	pwd, _ := os.Getwd()
	//文件夹存在的话会返回一个错误，可以用`_`抛出去
	err = os.Mkdir(pwd+upload_path, os.ModePerm)
	if err != nil {
		fmt.Println("dir is create Error")
	}
	fW, err := os.Create(pwd + upload_path + head.Filename)
	if err != nil {
		GVA_LOG.Error("文件创建失败:",zap.Any("err",err))
	}
	fmt.Println(*fW)
	defer fW.Close()
	//复制文件，保存到本地
	_, err = io.Copy(fW, file)
	if err != nil {
		GVA_LOG.Error("文件保存失败:",zap.Any("err",err))
	}
	imageUrl :=upload_qiniu(pwd + upload_path + head.Filename)
	//获取baidu-accessToken
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
		GVA_LOG.Error("请求百度ai接口失败:",zap.Any("err",err))
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
	//构建上传表单对象
	formUploader := storage.NewFormUploader(&cfg)
	ret := storage.PutRet{}
	// 可选
	putExtra := storage.PutExtra{
		Params: map[string]string{
			"x:name": "github logo",
		},
	}
	err := formUploader.PutFile(context.Background(), &ret, upToken, key, filePath, &putExtra)
	if err != nil {
		GVA_LOG.Error("图片上传七牛失败~:",zap.Any("err",err))
	}
	fmt.Println(ret.Key)
	imageUrl = GVA_CONFIG.Qiniu.ImgPath + ret.Key
	//fmt.Println(ret.Key, ret.Hash)
	return
}

//获取时间YYMMDDHis
func GetDateYMDHis()  string {
	now := time.Now().UTC()
	month := time.Unix(1557042972, 0).Format("1")
	year := now.Format("2006")
	month = now.Format("01")
	day := now.Format("02")
	//hour, min, sec := now.UTC().Clock()
	hour := strconv.Itoa(now.Hour())
	minute := strconv.Itoa(now.Minute())
	second := strconv.Itoa(now.Second())
	if len(month) == 1{
		month ="0"+month
	}
	if len(day) == 1{
		day = "0"+day
	}
	if len(hour) == 1{
		hour ="0"+hour
	}
	if len(minute) == 1{
		minute ="0"+minute
	}
	if len(second) == 1{
		second ="0"+second
	}

	date_time := year + month + day + hour + minute + second

	return date_time
}

// 获取百度AccessToken
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
		RefreshToken 	string 	`json:"refresh_token"`
		ExpiresIn  		uint  	`json:"expires_in"`
		SessionKey  	string 	`json:"session_key"`
		AccessToken		string	`json:"access_token"`
		Scope			string  `json:"scope"`
		SessionSecret	string	`json:"session_secret"`
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
	http.HandleFunc("/webimage", webimageInfo)
	http.HandleFunc("/webimage/uploadImage",uploadImage) //upload image~
	http.HandleFunc("/webimage/saveCard",saveCard) //save card~

	http.ListenAndServe(":"+GVA_CONFIG.System.Addr, nil)

}