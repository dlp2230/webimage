package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/idoubi/goz"
	"github.com/qiniu/api.v7/v7/auth/qbox"
	"github.com/qiniu/api.v7/v7/storage"
	"log"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"io"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"
	"webimage/asset"
	assetfs "github.com/elazarl/go-bindata-assetfs"
)

var (
	DBM *gorm.DB
	DBS *gorm.DB
)

//定义数据库连接
type ConnInfo struct {
	MyUser string
	Password string
	Host string
	Port int
	Db string
}


var myTemplate *template.Template
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

var (
	ACCESS_KEY = "CKzFTFiQ91LG0kin8_c-eOs8uCSxOn2HGykp5dmr"
	SECRET_KEY = "uCMwldThLJNb0wZsAZqxPVyZBY2pZ5SCmgP7uXBn"
	BUCKET     = "expocraftsmen"
	IMAGE_PATH = "https://expo-img.expocraftsmen.com/"
)

func init() {
	//正式数据库配置
	cn := ConnInfo{
		"applereservation",
		"Js3jjrMz2dWb7DnN",
		"47.88.2.29",
		3306,
		"applereservation",
	}

	DBS = DbConn(cn.MyUser,cn.Password,cn.Host,cn.Db,cn.Port)
	DBS.LogMode(true)
	DBM = DbConn(cn.MyUser,cn.Password,cn.Host,cn.Db,cn.Port)
	defer DBM.Close()
}

func DbConn(MyUser, Password, Host, Db string, Port int) *gorm.DB {
	connArgs := fmt.Sprintf("%s:%s@(%s:%d)/%s?charset=utf8&parseTime=True&loc=Local",  MyUser,Password, Host, Port, Db )
	db, err := gorm.Open("mysql", connArgs)
	if err != nil {
		log.Fatal(err)
	}
	db.SingularTable(true)
	return db
}

func webimageInfo(w http.ResponseWriter,r *http.Request) {
	bytes, err := asset.Asset("public/view/webimage.html")  // 根据地址获取对应内容
	if err != nil {
		panic(err)
	}
	t, err := template.New("index").Delims("<<", ">>").Parse(string(bytes))      // 比如用于模板处理

	t = template.Must(t, err)
	if err != nil {
		panic(err)
	}
	var fileList = make(map[string]interface{})

	t.Execute(w, fileList)

	//myTemplate.Execute(w, struct {
	//
	//}{})
}

func initTemplate(fileName string) (err error){
	myTemplate,err  = template.ParseFiles(fileName)
	if err != nil{
		fmt.Println("parse file err:",err)
		return
	}
	return
}

// home
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	p := &returnData{}
	p.Code = 200
	p.Msg = "success~"
	returnMap := make(map[string]interface{})
	p.Result = returnMap
	data, _ := json.Marshal(p)
	fmt.Fprintln(w,string(data))
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
		fmt.Println(err)
		return
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
		fmt.Println("文件创建失败")
		return
	}
	fmt.Println(*fW)
	defer fW.Close()
	//复制文件，保存到本地
	_, err = io.Copy(fW, file)
	if err != nil {
		fmt.Println("文件保存失败")
		return
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
	//if err != nil {
	//	global.GVA_LOG.Error("baidu ai-image request err", zap.Any("err", err))
	//}
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
		Scope: BUCKET,
	}
	mac := qbox.NewMac(ACCESS_KEY, SECRET_KEY)
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
		fmt.Println(err)
		return
	}
	fmt.Println(ret.Key)
	imageUrl = IMAGE_PATH + ret.Key
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
	postParam["client_id"] = "bLmOZEzmmAIHZGojuWx4Ie4z"
	postParam["client_secret"] = "f7B6WYBS8WOwwhTarmNWj0cq2WuH5QY5"
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

	//initTemplate("public/view/webimage.html")

	http.HandleFunc("/webimage", webimageInfo)
	http.HandleFunc("/webimage/uploadImage",uploadImage) //upload image~
	http.HandleFunc("/webimage/saveCard",saveCard) //save card~

	http.ListenAndServe(":8009", nil)

}