package gongxueyun_service

import (
	"BronyaBot/global"
	"BronyaBot/internal/api"
	"BronyaBot/internal/entity"
	"BronyaBot/internal/service/gongxueyun_service/data"
	"BronyaBot/utils"
	"BronyaBot/utils/blockPuzzle"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

type MoguDing struct {
	ID            int        `json:"ID"`
	UserId        string     `json:"userId"`
	RoleKey       string     `json:"roleKey"`
	Authorization string     `json:"authorization"`
	PlanID        string     `json:"planId"`
	PlanName      string     `json:"planName"`
	PhoneNumber   string     `json:"phoneNumber"`
	Password      string     `json:"password"`
	Sign          SignStruct `json:"sign"`
	Email         string     `json:"email"`
}

func (m *MoguDing) Run() {
	global.Log.Infof("Starting sign-in process for user: %s", m.PhoneNumber)
	//m.GetBlock()
	if err := m.GetBlock(); err != nil {
		utils.SendMail(m.Email, "Block-Error", err.Error())
		global.Log.Error(err.Error())
		return
	}

	if err := m.Login(); err != nil {
		utils.SendMail(m.Email, "Login-Error-测试邮件请勿回复", err.Error())
		global.Log.Error(err.Error())
		return
	}
	m.GetPlanId()
	m.SignIn()
	//m.getSubmittedReportsInfo("week")
}

type SignStruct struct {
	//	构造签到信息
	Address   string `json:"address"`
	City      string `json:"city"`
	Area      string `json:"area"`
	Country   string `json:"country"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
	Province  string `json:"province"`
}
type commonParameters struct {
	token     string
	secretKey string
	xY        string
	captcha   string
}

var headers = map[string][]string{
	"User-Agent":   {"Mozilla/5.0 (Linux; U; Android 9; zh-cn; Redmi Note 5 Build/PKQ1.180904.001) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/71.0.3578.141 Mobile Safari/537.36 XiaoMi/MiuiBrowser/11.10.8"},
	"Content-Type": {"application/json; charset=UTF-8"},
	"host":         {"api.moguding.net:9000"},
}
var clientUid = strings.ReplaceAll(uuid.New().String(), "-", "")
var comm = &commonParameters{}

func addHeader(key, value string) {
	// 检查 key 是否已经存在，若存在则追加到对应的值
	if _, exists := headers[key]; exists {
		//headers[key] = append(headers[key], value)
		headers[key] = []string{value}
	} else {
		// 若不存在，则新建一个字段
		headers[key] = []string{value}
	}
}
func GenerateRandomFloat(baseIntegerPart int) float64 {
	rand.Seed(time.Now().UnixNano())

	// Randomly adjust the integer part by ±1
	adjustment := rand.Intn(4) - 1 // Generates -1, 0, or 1
	integerPart := baseIntegerPart + adjustment

	// Calculate the maximum number of decimal places based on the integer part's length
	intPartLength := len(fmt.Sprintf("%d", integerPart))
	totalLength := rand.Intn(10) + 10 // Total length between 10 and 19
	decimalPlaces := totalLength - intPartLength

	if decimalPlaces <= 0 {
		decimalPlaces = 1 // Ensure at least one decimal place
	}

	// Generate a random decimal value with the specified number of decimal places
	decimalPart := rand.Float64() * math.Pow(10, float64(decimalPlaces))
	decimalPart = math.Trunc(decimalPart) / math.Pow(10, float64(decimalPlaces)) // Truncate to avoid floating-point imprecision

	return float64(integerPart) + decimalPart
}
func (mo *MoguDing) GetBlock() error {
	var maxRetries = 15
	for attempts := 1; attempts <= maxRetries; attempts++ {
		err := mo.processBlock()
		if err == nil {
			return nil
		}
		global.Log.Warning(fmt.Sprintf("Retrying captcha (%d/%d)", attempts, maxRetries))
		time.Sleep(10 * time.Second)
	}
	global.Log.Error("Failed to process captcha after maximum retries")
	return fmt.Errorf("failed to process captcha after maximum retries")
}
func (mo *MoguDing) processBlock() error {
	// 获取验证码数据
	requestData := map[string]interface{}{
		"clientUid":   clientUid,
		"captchaType": "blockPuzzle",
	}
	body, err := utils.SendRequest("POST", api.BaseApi+api.BlockPuzzle, requestData, headers)
	if err != nil {
		return fmt.Errorf("failed to fetch block puzzle: %v", err)
	}
	// 解析响应数据
	blockData := &data.BlockRes{}
	if err := json.Unmarshal(body, &blockData); err != nil {
		return fmt.Errorf("failed to parse block puzzle response: %v", err)
	}

	// 初始化滑块验证码
	captcha, err := blockPuzzle.NewSliderCaptcha(blockData.Data.JigsawImageBase64, blockData.Data.OriginalImageBase64)
	if err != nil {
		return fmt.Errorf("failed to initialize captcha: %v", err)
	}
	x, _ := captcha.FindBestMatch()

	// 加密并验证
	xY := map[string]string{"x": strconv.FormatFloat(GenerateRandomFloat(x), 'f', -1, 64), "y": strconv.Itoa(5)}
	global.Log.Info(fmt.Sprintf("Captcha matched at: xY=%s", xY))

	marshal, err := json.Marshal(xY)
	comm.xY = string(marshal)
	comm.token = blockData.Data.Token
	comm.secretKey = blockData.Data.SecretKey
	cipher, _ := utils.NewAESECBPKCS5Padding(comm.secretKey, "base64")
	encrypt, _ := cipher.Encrypt(comm.xY)
	requestData = map[string]interface{}{
		"pointJson":   encrypt,
		"token":       blockData.Data.Token,
		"captchaType": "blockPuzzle",
	}
	body, err = utils.SendRequest("POST", api.BaseApi+api.CHECK, requestData, headers)
	if err != nil {
		return fmt.Errorf("failed to verify captcha: %v", err)
	}

	// 解析验证结果
	jsonContent := &data.CheckData{}
	if err := json.Unmarshal(body, &jsonContent); err != nil {
		return fmt.Errorf("failed to parse check response: %v", err)
	}
	if jsonContent.Code == 6111 {
		return fmt.Errorf("captcha verification failed, retry needed")
	}
	global.Log.Info("Captcha verification successful")
	padding, _ := utils.NewAESECBPKCS5Padding(blockData.Data.SecretKey, "base64")
	encrypt, err = padding.Encrypt(jsonContent.Data.Token + "---" + comm.xY)
	if err != nil {
		global.Log.Info(fmt.Sprintf("Failed to encrypt captcha: %v", err))
	}
	comm.captcha = encrypt
	return nil
}
func (mogu *MoguDing) Login() error {
	padding, _ := utils.NewAESECBPKCS5Padding(utils.MoGuKEY, "hex")
	encryptPhone, _ := padding.Encrypt(mogu.PhoneNumber)
	encryptPassword, _ := padding.Encrypt(mogu.Password)
	timestamp, _ := encryptTimestamp(time.Now().UnixMilli())
	requestData := map[string]interface{}{
		"phone":     encryptPhone,
		"password":  encryptPassword,
		"captcha":   comm.captcha,
		"loginType": "android",
		"uuid":      clientUid,
		"device":    "android",
		"version":   "5.15.0",
		"t":         timestamp,
	}
	var login = &data.Login{}
	var loginData = &data.LoginData{}
	body, err := utils.SendRequest("POST", api.BaseApi+api.LoginAPI, requestData, headers)
	if err != nil {
		global.Log.Info(fmt.Sprintf("Failed to send request: %v", err))
	}
	json.Unmarshal(body, &login)
	if login.Code != 200 {
		return fmt.Errorf(login.Msg)

	}
	decrypt, err := padding.Decrypt(login.Data)
	json.Unmarshal([]byte(decrypt), &loginData)
	if err != nil {
		global.Log.Info(fmt.Sprintf("Failed to decrypt data: %v", err))
	}
	mogu.RoleKey = loginData.RoleKey
	mogu.UserId = loginData.UserId
	mogu.Authorization = loginData.Token
	global.Log.Info("================")
	global.Log.Info(loginData.NikeName)
	global.Log.Info(loginData.Phone)
	global.Log.Info("================")
	global.Log.Info("Login successful")
	return nil
}
func (mogu *MoguDing) GetPlanId() {
	planData := &data.PlanByStuData{}
	timestamp, _ := encryptTimestamp(time.Now().UnixMilli())
	sign := utils.CreateSign(mogu.UserId, mogu.RoleKey)
	addHeader("rolekey", mogu.RoleKey)
	addHeader("sign", sign)
	addHeader("authorization", mogu.Authorization)
	body := map[string]interface{}{
		"pageSize": strconv.Itoa(999999),
		"t":        timestamp,
	}
	request, err := utils.SendRequest("POST", api.BaseApi+api.GetPlanIDAPI, body, headers)
	if err != nil {
		global.Log.Info(fmt.Sprintf("Failed to send request: %v", err))
	}
	json.Unmarshal(request, &planData)
	for i := range planData.Data {
		mogu.PlanID = planData.Data[i].PlanId
		mogu.PlanName = planData.Data[i].PlanName
	}
	global.Log.Info("================")
	global.Log.Info(mogu.PlanID)
	global.Log.Info(mogu.PlanName)
	global.Log.Info("================")
}
func (mogu *MoguDing) SignIn() {
	resdata := &data.SaveData{}
	filling := dataStructureFilling(mogu)
	sign := utils.CreateSign(filling["device"].(string), filling["type"].(string), mogu.PlanID, mogu.UserId, filling["address"].(string))
	addHeader("rolekey", mogu.RoleKey)
	addHeader("sign", sign)
	addHeader("authorization", mogu.Authorization)
	request, err := utils.SendRequest("POST", api.BaseApi+api.SignAPI, filling, headers)
	if err != nil {
		global.Log.Info(fmt.Sprintf("Failed to send request: %v", err))
	}

	json.Unmarshal(request, &resdata)
	global.Log.Info("================")
	global.Log.Info(resdata.Msg)
	global.Log.Info("================")
	if resdata.Msg == "success" {
		mogu.updateSignState(1)
	} else {
		mogu.updateSignState(0)
	}
	utils.SendMail(mogu.Email, "检查是否打卡完成", resdata.Msg+"\n如果未成功请联系管理员")

}
func (mogu *MoguDing) updateSignState(state int) {
	// 更新数据库表中的 state 字段
	err := global.DB.Model(&entity.SignEntity{}).Where("username = ?", mogu.PhoneNumber).Update("state", state).Error
	if err != nil {
		global.Log.Error(fmt.Sprintf("Failed to update state for user %s: %v", mogu.PhoneNumber, err))
	} else {
		global.Log.Info(fmt.Sprintf("Successfully updated state for user %s to %d", mogu.PhoneNumber, state))
	}
}

// 获取已经提交的日报、周报或月报的数量。
func (mogu *MoguDing) getSubmittedReportsInfo(reportType string) {
	report := &data.ReportsInfo{}
	sign := utils.CreateSign(mogu.UserId, mogu.RoleKey, reportType)
	addHeader("rolekey", mogu.RoleKey)
	addHeader("userid", mogu.UserId)
	addHeader("sign", sign)
	timestamp, _ := encryptTimestamp(time.Now().UnixMilli())
	body := map[string]interface{}{
		"currPage":   1,
		"pageSize":   10,
		"reportType": reportType,
		"planId":     mogu.PlanID,
		"t":          timestamp,
	}
	request, err := utils.SendRequest("POST", api.BaseApi+api.GetWeekCountAPI, body, headers)
	if err != nil {
		global.Log.Info(fmt.Sprintf("Failed to send request: %v", err))
	}
	json.Unmarshal(request, &report)
	global.Log.Info(report)
}
func dataStructureFilling(mogu *MoguDing) map[string]interface{} {
	// 加载中国时区
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		global.Log.Error("Failed to load location: ", err)
		return nil
	}
	// 获取当前时间并格式化
	now := time.Now().In(loc)
	formattedTime := now.Format("2006-01-02 15:04:05")

	// 确定打卡类型
	typeStr := "START"
	if now.Hour() >= 12 {
		typeStr = "END"
	}
	// 加密当前时间戳
	encryptTime, err := encryptTimestamp(now.UnixMilli())
	if err != nil {
		global.Log.Error("Failed to encrypt timestamp: ", err)
		return nil
	}
	// 直接构造 map，而不是先构造结构体再转换为 map
	return map[string]interface{}{
		"address":    mogu.Sign.Address,
		"city":       mogu.Sign.City,
		"area":       mogu.Sign.Area,
		"country":    mogu.Sign.Country,
		"createTime": formattedTime,
		"device":     "{brand: Redmi Note 5, systemVersion: 14, Platform: Android}",
		"latitude":   mogu.Sign.Latitude,
		"longitude":  mogu.Sign.Longitude,
		"province":   mogu.Sign.Province,
		"state":      "NORMAL",
		"type":       typeStr,
		"userId":     mogu.UserId,
		"t":          encryptTime,
		"planId":     mogu.PlanID,
	}
}

// 加密时间戳的通用方法
func encryptTimestamp(timestamp int64) (string, error) {
	padding, err := utils.NewAESECBPKCS5Padding(utils.MoGuKEY, "hex")
	if err != nil {
		return "", fmt.Errorf("failed to initialize padding: %v", err)
	}

	encryptTime, err := padding.Encrypt(strconv.FormatInt(timestamp, 10))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt timestamp: %v", err)
	}
	return encryptTime, nil
}
