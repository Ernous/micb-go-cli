package micb

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	BaseURL     = "https://wb.micb.md"
	MobileAPI   = BaseURL + "/mobile2/api/v2"
	APIGate     = BaseURL + "/api-gate"
	APIKey      = "7b9b7649-f8ed-407e-a029-3e083c3e2d5c"
	AppVersion  = "1.6.5850.3"
	APIAppVer   = "v1-1.6.5850.3"
	UserAgent   = "micb-go-cli/1.0"
)

var DefaultDeviceID string

func init() {
	DefaultDeviceID = uuid.New().String()
}

type Client struct {
	http       *http.Client
	jar        *cookiejar.Jar
	login      string
	userID     string
	extToken   string
	mmaPIN     string
	mmaProf    *PersonalizationProfile
	deviceID   string
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{Jar: jar},
		jar:  jar,
	}
}

func (c *Client) SetLogin(login string) { c.login = login }

func (c *Client) SetMMACredentials(pin string, prof *PersonalizationProfile) {
	c.mmaPIN = pin
	c.mmaProf = prof
}

func (c *Client) HasMMACredentials() bool {
	return c.mmaPIN != "" && c.mmaProf != nil
}

func (c *Client) MMAProfile() *PersonalizationProfile {
	return c.mmaProf
}

func (c *Client) RefreshSession() error {
	if !c.HasMMACredentials() {
		return fmt.Errorf("no MMA credentials stored")
	}
	password, newTC, err := GenerateSessionPassword(c.mmaPIN, *c.mmaProf)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	c.mmaProf.TransactionCounter = newTC

	sr, err := c.LoginMMA(LoginMMARequest{
		LoginMethod:   "LOGIN_MMA",
		Login:         c.mmaProf.Login,
		Password:      password,
		AuthType:      "otp_mma",
		ApplicationID: c.mmaProf.ID,
	})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if sr.User == nil {
		return fmt.Errorf("refresh failed: %v", sr)
	}

	if err := c.CreateAPISession(); err != nil {
		return fmt.Errorf("api session: %w", err)
	}
	return nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", UserAgent)
	return c.http.Do(req)
}

func (c *Client) mobileReq(method, path string, body interface{}) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, MobileAPI+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("ow-client-version", AppVersion)
	req.Header.Set("x-micb-ip", getOutboundIP())
	return c.do(req)
}

func (c *Client) gateReq(method, path string, body interface{}) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, APIGate+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("x-micb-api", APIKey)
	req.Header.Set("x-micb-from", "mobile")
	req.Header.Set("x-micb-app-version", APIAppVer)
	req.Header.Set("x-micb-deviceid", c.GetDeviceID())
	if c.login != "" {
		req.Header.Set("x-micb-login", c.login)
	}
	return c.do(req)
}

func (c *Client) GetCaptcha() ([]byte, error) {
	resp, err := c.mobileReq("GET", "/captcha.jpg", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

type SessionResponse struct {
	Status          string `json:"status"`
	CaptchaRequired bool   `json:"captchaRequired"`
	LoginMethods    map[string]struct {
		Default bool `json:"default"`
	} `json:"loginMethods"`
	Status2       string       `json:"_status"`
	Confirmation  *Confirmation `json:"confirmation"`
	ExpiresInMsec int          `json:"expiresInMsec"`
	AuthType      string       `json:"authType"`
	User          *User        `json:"user"`
	ExtSessionToken string     `json:"extSessionToken"`
}

type Confirmation struct {
	ConfirmationKey string   `json:"confirmationKey"`
	AuthType        string   `json:"authType"`
	SmsTemplate     string   `json:"smsTemplate"`
	Challenge       struct {
		TriesLeft int    `json:"triesLeft"`
		Phone     string `json:"phone"`
	} `json:"challenge"`
	Response string `json:"response,omitempty"`
}

type User struct {
	ID            string         `json:"id"`
	Login         string         `json:"login"`
	DisplayName   string         `json:"displayName"`
	W4cClientID   string         `json:"w4cClientId"`
	MobileDevices []MobileDevice `json:"mobileDevices"`
}

type MobileDevice struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PersonalizedAt string `json:"personalizedAt"`
}

func (c *Client) SetDeviceID(id string) {
	c.deviceID = id
}

func (c *Client) GetDeviceID() string {
	if c.deviceID != "" {
		return c.deviceID
	}
	return DefaultDeviceID
}

func (c *Client) GetMobileDevices() ([]MobileDevice, error) {
	sr, err := c.CheckSession()
	if err != nil {
		return nil, err
	}
	if sr.User == nil {
		return nil, fmt.Errorf("не аутентифицирован")
	}
	return sr.User.MobileDevices, nil
}

func (c *Client) CheckSession() (*SessionResponse, error) {
	resp, err := c.mobileReq("GET", "/session", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sr SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

type LoginRequest struct {
	LoginMethod string `json:"loginMethod"`
	Login       string `json:"login"`
	Password    string `json:"password"`
	Captcha     string `json:"captcha"`
}

type LoginMMARequest struct {
	LoginMethod   string `json:"loginMethod"`
	Login         string `json:"login"`
	Password      string `json:"password"`
	Captcha       string `json:"captcha,omitempty"`
	AuthType      string `json:"authType"`
	AuthMethod    string `json:"authMethod,omitempty"`
	ApplicationID string `json:"applicationId"`
}

func (c *Client) Login(req LoginRequest) (*SessionResponse, error) {
	resp, err := c.mobileReq("POST", "/session", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sr SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	if sr.User != nil {
		c.userID = sr.User.ID
		c.extToken = sr.ExtSessionToken
		c.login = sr.User.Login
	}

	return &sr, nil
}

func (c *Client) LoginMMA(req LoginMMARequest) (*SessionResponse, error) {
	resp, err := c.mobileReq("POST", "/session", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var sr SessionResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}

	if sr.User != nil {
		c.userID = sr.User.ID
		c.extToken = sr.ExtSessionToken
		c.login = sr.User.Login
	}

	return &sr, nil
}

type ConfirmOTPRequest struct {
	LoginMethod  string       `json:"loginMethod"`
	Login        string       `json:"login"`
	Password     string       `json:"password"`
	Captcha      string       `json:"captcha,omitempty"`
	Status       *string      `json:"_status"`
	Confirmation Confirmation `json:"confirmation"`
}

func (c *Client) ConfirmOTP(req ConfirmOTPRequest) (*SessionResponse, error) {
	resp, err := c.mobileReq("POST", "/session", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sr SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	if sr.User != nil {
		c.userID = sr.User.ID
		c.extToken = sr.ExtSessionToken
		c.login = sr.User.Login
	}

	return &sr, nil
}

func (c *Client) CreateAPISession() error {
	auth := fmt.Sprintf("%s:%s", c.userID, c.extToken)
	encoded := base64Encode(auth)

	req, err := http.NewRequest("POST", APIGate+"/user-id/session/create", bytes.NewReader([]byte(`{"clientId":"`+c.userID+`"}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("x-micb-api", APIKey)
	req.Header.Set("x-micb-from", "mobile")
	req.Header.Set("x-micb-app-version", APIAppVer)
	req.Header.Set("x-micb-opname", "session-create")
	req.Header.Set("x-micb-login", c.login)
	req.Header.Set("Authorization", "Basic "+encoded)
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if s, ok := result["success"]; !ok || s != true {
		return fmt.Errorf("session create failed: %v", result)
	}
	return nil
}

func (c *Client) GetUser() (*User, error) {
	resp, err := c.mobileReq("GET", "/user", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) GetContracts() ([]map[string]interface{}, error) {
	resp, err := c.mobileReq("GET", "/dashboard/contracts?system=W4C&filter=product.id__neq:LIAB", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("server returned HTML error page instead of JSON (status %d)", resp.StatusCode)
	}

	// Server returns a JSON array [{...}, {...}]
	var arr []map[string]interface{}
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}

	// Fallback: try object with profile.additionalCards.cards
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	profile, _ := result["profile"].(map[string]interface{})
	if profile == nil {
		return nil, nil
	}
	addCards, _ := profile["additionalCards"].(map[string]interface{})
	if addCards == nil {
		return nil, nil
	}
	cards, _ := addCards["cards"].([]interface{})
	var out []map[string]interface{}
	for _, c := range cards {
		if cm, ok := c.(map[string]interface{}); ok {
			out = append(out, cm)
		}
	}
	return out, nil
}

func (c *Client) GetAccounts() ([]map[string]interface{}, error) {
	return c.GetContracts()
}

type cardReqRequest struct {
	Modulus    string `json:"modulus"`
	ID         string `json:"id"`
	DeviceID   string `json:"deviceId,omitempty"`
	DigitalCard *bool  `json:"digitalCard,omitempty"`
	Processing string `json:"processing,omitempty"`
}

type cardReqResponse struct {
	Encoded string `json:"encoded"`
}

func generateRSAKey() (*rsa.PrivateKey, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, "", err
	}
	// The app encodes modulus as base64 of the hex string (btoa(hexString))
	hexMod := hex.EncodeToString(key.N.Bytes())
	n := base64.StdEncoding.EncodeToString([]byte(hexMod))
	return key, n, nil
}

func decryptCardData(key *rsa.PrivateKey, encoded string) (string, error) {
	hexStr, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	c := new(big.Int)
	if _, ok := c.SetString(string(hexStr), 16); !ok {
		return "", fmt.Errorf("invalid ciphertext hex")
	}
	m := crtDecrypt(key, c)
	if m == nil {
		return "", fmt.Errorf("decrypt failed")
	}
	plain := pkcs1unpad(m, key.N.BitLen())
	if plain == nil {
		return "", fmt.Errorf("pkcs1 unpad failed")
	}
	return string(plain), nil
}

func crtDecrypt(key *rsa.PrivateKey, c *big.Int) *big.Int {
	if len(key.Primes) < 2 {
		return new(big.Int).Exp(c, key.D, key.N)
	}
	p := key.Primes[0]
	q := key.Primes[1]
	dmp1 := key.Precomputed.Dp
	dmq1 := key.Precomputed.Dq
	coeff := key.Precomputed.Qinv

	t := new(big.Int).Mod(c, p)
	t.Exp(t, dmp1, p)

	n := new(big.Int).Mod(c, q)
	n.Exp(n, dmq1, q)

	for t.Cmp(n) < 0 {
		t.Add(t, p)
	}
	r := new(big.Int).Sub(t, n)
	r.Mul(r, coeff)
	r.Mod(r, p)
	r.Mul(r, q)
	r.Add(r, n)
	return r
}

func pkcs1unpad(m *big.Int, bitLen int) []byte {
	blockLen := (bitLen + 7) >> 3
	raw := m.Bytes()
	// Pad with leading zeros to blockLen
	if len(raw) < blockLen {
		padded := make([]byte, blockLen)
		copy(padded[blockLen-len(raw):], raw)
		raw = padded
	}
	// Find start offset (skip leading zeros)
	i := 0
	for i < len(raw) && raw[i] == 0 {
		i++
	}
	// Check: remaining must be blockLen-1 and first byte must be 2 (PKCS1 type 2)
	if len(raw)-i != blockLen-1 || raw[i] != 2 {
		return nil
	}
	i++ // skip type byte
	// Skip padding bytes (non-zero until 0x00 separator)
	for i < len(raw) && raw[i] != 0 {
		i++
	}
	if i >= len(raw) {
		return nil
	}
	i++ // skip separator
	// Return everything after separator
	return raw[i:]
}

var cardReqOpnames = map[string]string{
	"card-req/v2/number": "card-req-number",
	"card-req/v2/cvv":    "card-req-cvv",
}

func (c *Client) cardReq(path string, req cardReqRequest) (*cardReqResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest("POST", BaseURL+"/"+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("User-Agent", UserAgent)
	httpReq.Header.Set("x-micb-opname", cardReqOpnames[path])
	httpReq.Header.Set("x-micb-from", "mobile")
	httpReq.Header.Set("x-micb-rqUid", uuid.New().String())
	httpReq.Header.Set("x-micb-mobile-sdk-data", "7560a142d7cd84a02058ae7f748fb56987f7032da5a5b1186e999319c6b9ef67")
	httpReq.Header.Set("x-app-lang", "ro")
	httpReq.Header.Set("Accept-Language", "ro")
	httpReq.Header.Set("x-micb-api", APIKey)
	httpReq.Header.Set("x-micb-deviceid", c.GetDeviceID())
	httpReq.Header.Set("x-micb-device-manufacturer", "Xiaomi")
	httpReq.Header.Set("x-micb-device-model", "220333QPG")
	httpReq.Header.Set("x-micb-device-platform", "Android")
	httpReq.Header.Set("x-micb-os-version", "14")
	httpReq.Header.Set("x-micb-device-screen-resolution", "450x1032")
	httpReq.Header.Set("x-micb-app-version", APIAppVer)
	if c.login != "" {
		httpReq.Header.Set("x-micb-login", c.login)
	}
	// card-req endpoints live under root path, but cookies have path=/mobile2/api/v2 or /api-gate
	// Copy JSESSIONID and SUB-SESSION-ID from cookies so they're sent with card-req requests
	mu, _ := url.Parse(MobileAPI)
	for _, cookie := range c.jar.Cookies(mu) {
		if cookie.Name == "JSESSIONID" || cookie.Name == "SUB-SESSION-ID" {
			httpReq.AddCookie(cookie)
		}
	}
	gu, _ := url.Parse(APIGate)
	for _, cookie := range c.jar.Cookies(gu) {
		if cookie.Name == "SUB-SESSION-ID" {
			httpReq.AddCookie(cookie)
		}
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusPreconditionRequired {
		var scaErr struct {
			IsValid      bool `json:"isValid"`
			ConfirmTypes *struct {
				Action       string   `json:"action"`
				ConfirmTypes []string `json:"confirmTypes"`
			} `json:"confirmTypes"`
		}
		if json.Unmarshal(body, &scaErr) == nil && scaErr.ConfirmTypes != nil {
			return nil, fmt.Errorf("требуется подтверждение в приложении: открой официальное приложение MICB и подтверди запрос (PIN/biometrics), затем повтори команду")
		}
		return nil, fmt.Errorf("card-req error (428): %s", string(body))
	}

	var r cardReqResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("card-req parse: %w (body=%s)", err, string(body))
	}
	if r.Encoded == "" {
		var serr struct {
			Exception string `json:"exceptionType"`
			Message   string `json:"message"`
		}
		if json.Unmarshal(body, &serr) == nil && serr.Message != "" {
			return nil, fmt.Errorf("%s", serr.Message)
		}
		return nil, fmt.Errorf("card-req error: %s", string(body))
	}
	return &r, nil
}

func (c *Client) GetNotifications() ([]map[string]interface{}, error) {
	resp, err := c.gateReq("GET", "/feed/api/v1/announcements/messages/all?page=0&size=20", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Announcements []map[string]interface{} `json:"announcements"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("announcements parse: %w (body=%s)", err, string(body))
	}
	return result.Announcements, nil
}

func (c *Client) GetCardNumber(cbsNumber string) (string, error) {
	key, modulus, err := generateRSAKey()
	if err != nil {
		return "", fmt.Errorf("rsa key: %w", err)
	}
	resp, err := c.cardReq("card-req/v2/number", cardReqRequest{
		Modulus: modulus,
		ID:      cbsNumber,
	})
	if err != nil {
		return "", err
	}
	return decryptCardData(key, resp.Encoded)
}

func (c *Client) initSCA(action string) {
	scaURL := BaseURL + "/card-req/confirmation/sca?action=" + action
	req, _ := http.NewRequest("POST", scaURL, nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("x-micb-api", APIKey)
	req.Header.Set("x-micb-from", "mobile")
	req.Header.Set("x-micb-app-version", APIAppVer)
	req.Header.Set("x-micb-deviceid", c.GetDeviceID())
	req.Header.Set("x-micb-confirmation", "ticket-id")
	req.Header.Set("x-micb-rquid", uuid.New().String())
	req.Header.Set("x-app-lang", "ru")
	req.Header.Set("accept-language", "ru")
	if c.login != "" {
		req.Header.Set("x-micb-login", c.login)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
}

func (c *Client) SaveCookies(path string) error {
	u, _ := url.Parse(BaseURL)
	data, err := json.Marshal(c.jar.Cookies(u))
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Save session data along with cookies
	sessionData := map[string]interface{}{
		"cookies":   string(data),
		"userID":    c.userID,
		"extToken":  c.extToken,
		"login":     c.login,
	}
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return err
	}
	return os.WriteFile(path, sessionJSON, 0600)
}

func (c *Client) LoadCookies(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var sessionData map[string]interface{}
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return err
	}

	// Load cookies
	if cookiesStr, ok := sessionData["cookies"].(string); ok {
		var cookies []*http.Cookie
		if err := json.Unmarshal([]byte(cookiesStr), &cookies); err == nil {
			u, _ := url.Parse(BaseURL)
			c.jar.SetCookies(u, cookies)
		}
	}

	// Load session fields
	if userID, ok := sessionData["userID"].(string); ok {
		c.userID = userID
	}
	if extToken, ok := sessionData["extToken"].(string); ok {
		c.extToken = extToken
	}
	if login, ok := sessionData["login"].(string); ok {
		c.login = login
	}

	return nil
}

type DeviceNonceResponse struct {
	RequestID          string `json:"requestId"`
	ChallengeBase64URL string `json:"challengeBase64Url"`
}

func (c *Client) GetDeviceNonce() (*DeviceNonceResponse, error) {
	resp, err := c.gateReq("GET", "/settings/device/nonce", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var nr DeviceNonceResponse
	if err := json.NewDecoder(resp.Body).Decode(&nr); err != nil {
		return nil, err
	}
	return &nr, nil
}

type SecurityEvent struct {
	Accessibility struct {
		HasUntrusted bool     `json:"hasUntrusted"`
		Enabled      []string `json:"enabled"`
		Allowlist    []string `json:"allowlist"`
	} `json:"accessibility"`
	Overlay     bool `json:"overlay"`
	DeviceAdmin struct {
		HasOtherAdmins    bool `json:"hasOtherAdmins"`
		ActiveAdminsCount int  `json:"activeAdminsCount"`
	} `json:"deviceAdmin"`
	Root           bool `json:"root"`
	SignatureValid bool `json:"signatureValid"`
	Debug          struct {
		SDKInt       int    `json:"sdkInt"`
		Manufacturer string `json:"manufacturer"`
		Model        string `json:"model"`
	} `json:"debug"`
}

func (c *Client) SendSecurityEvent(event SecurityEvent) error {
	req, err := http.NewRequest("POST", APIGate+"/crashlytics/security/event", nil)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(event)
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("x-micb-api", APIKey)
	req.Header.Set("x-micb-from", "mobile")
	req.Header.Set("x-micb-app-version", APIAppVer)
	req.Header.Set("x-micb-opname", "security-event")
	req.Header.Set("x-micb-internal", "true")
	req.Header.Set("User-Agent", UserAgent)
	req.ContentLength = int64(len(data))

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) SaveProfile(path string, profile PersonalizationProfile) error {
	data, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Client) LoadProfile(path string) (*PersonalizationProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile PersonalizationProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// EnrollMMAStep1 initiates enrollment and returns confirmationKey (SMS sent)
func (c *Client) EnrollMMAStep1(deviceID, label, pin string) (confirmationKey string, phone string, err error) {
	reqBody := map[string]interface{}{
		"deviceId": deviceID,
		"label":    label,
		"pin":      pin,
	}
	resp, err := c.mobileReq("POST", "/user/personalized-devices/enroll", reqBody)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", err
	}

	if status, ok := result["_status"].(string); ok && status == "confirmationRequired" {
		conf, _ := result["confirmation"].(map[string]interface{})
		key, _ := conf["confirmationKey"].(string)
		challenge, _ := conf["challenge"].(map[string]interface{})
		ph, _ := challenge["phone"].(string)
		return key, ph, nil
	}

	return "", "", fmt.Errorf("unexpected response: %s", string(body))
}

// EnrollMMAStep2 submits OTP and returns the profile
func (c *Client) EnrollMMAStep2(deviceID, label, pin, otp, confirmationKey string) (*PersonalizationProfile, error) {
	reqBody := map[string]interface{}{
		"deviceId": deviceID,
		"label":    label,
		"pin":      pin,
		"_status":  nil,
		"confirmation": map[string]interface{}{
			"confirmationKey": confirmationKey,
			"authType":       "otp_sms",
			"response":       otp,
		},
	}

	resp, err := c.mobileReq("POST", "/user/personalized-devices/enroll", reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if status, ok := result["_status"].(string); ok && status == "confirmationRequired" {
		return nil, fmt.Errorf("OTP rejected or expired")
	}

	var profile PersonalizationProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("decode profile: %w", err)
	}
	return &profile, nil
}

func (c *Client) Logout(cookieFile string) error {
	c.userID = ""
	c.extToken = ""
	c.login = ""
	return c.SaveCookies(cookieFile)
}

func (c *Client) IsAuthenticated() bool {
	return c.extToken != "" && c.userID != ""
}

func base64Encode(s string) string {
	encoder := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte
	data := []byte(s)
	for i := 0; i < len(data); i += 3 {
		b0 := data[i]
		b1 := byte(0)
		b2 := byte(0)
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}
		result = append(result, encoder[b0>>2])
		result = append(result, encoder[((b0&3)<<4)|(b1>>4)])
		if i+1 < len(data) {
			result = append(result, encoder[((b1&15)<<2)|(b2>>6)])
		} else {
			result = append(result, '=')
		}
		if i+2 < len(data) {
			result = append(result, encoder[b2&63])
		} else {
			result = append(result, '=')
		}
	}
	return string(result)
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}
