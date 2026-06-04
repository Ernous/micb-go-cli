package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"html"

	"micb-go-cli/pkg/micb"

	"github.com/chzyer/readline"
)

var (
	configDir   string
	cookieFile  string
	profileFile string
	client      *micb.Client
)

func init() {
	home, _ := os.UserHomeDir()
	configDir = filepath.Join(home, ".micb-cli")
	cookieFile = filepath.Join(configDir, "cookies.json")
	profileFile = filepath.Join(configDir, "mma_profile.json")
	client = micb.NewClient()
}

func main() {
	os.MkdirAll(configDir, 0700)
	client.LoadCookies(cookieFile)

	fmt.Println("╔══════════════════════════════════╗")
	fmt.Println("║     MICB Mobile Banking CLI      ║")
	fmt.Println("╚══════════════════════════════════╝")

	// Verify session with server
	if client.IsAuthenticated() {
		sr, err := client.CheckSession()
		if err != nil || sr.Status != "authenticated" {
			client.Logout(cookieFile)
		} else {
			// Refresh API gateway session
			if err := client.CreateAPISession(); err != nil {
				fmt.Printf("  предупреждение: сессия не обновлена: %v\n", err)
			}
		}
	}

	if client.IsAuthenticated() {
		fmt.Println("  ✓ сессия активна")
	} else if hasMMAProfile() {
		fmt.Println("  ○ не аутентифицированы (введи pin-login для быстрого входа по PIN)")
	} else {
		fmt.Println("  ○ не аутентифицированы")
	}
	fmt.Println()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:                 "micb> ",
		HistoryFile:            filepath.Join(configDir, "history"),
		DisableAutoSaveHistory: false,
	})
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "login":
			cmdLogin()
		case "user", "me":
			cmdUser()
		case "accounts", "acc":
			cmdAccounts()
		case "cards":
			cmdCards()
		case "card-info":
			cmdCardInfo(args)
		case "card-number":
			cmdCardNumber(args)
		case "notifications", "notif":
			cmdNotifications()
		case "captcha":
			cmdCaptcha()
		case "device":
			cmdDevice()
		case "devices":
			cmdDevices()
		case "device-use":
			cmdDeviceUse(args)
		case "mma-setup":
			cmdMMASetup()
		case "pin-login":
			cmdPinLogin(args)
		case "logout":
			cmdLogout()
		case "status":
			cmdStatus()
		case "help", "?":
			printHelp()
		case "exit", "quit", "q":
			fmt.Println("пока!")
			return
		default:
			fmt.Printf("  неизвестная команда: %s (help для списка)\n", cmd)
		}
		fmt.Println()
	}
}

func printHelp() {
	fmt.Println(`
Команды:
  login      Войти по логину и паролю
  mma-setup  Настроить MMA (профиль + PIN)
  pin-login  Быстрый вход по PIN (MMA)
  logout     Выйти из аккаунта
  user       Информация о пользователе
  accounts   Список счетов
  cards      Список карт
  card-info  Детали карты
   card-number <N>  Показать полный номер карты
  notifications    Список уведомлений
  captcha    Скачать капчу
  device     Эмуляция проверки устройства
  status     Статус сессии
  help       Эта справка
  exit       Выйти из программы

Сокращения: acc=accounts, me=user, notif=notifications, q=exit`)
}

func cmdStatus() {
	sr, err := client.CheckSession()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	fmt.Printf("  статус: %s\n", sr.Status)
	// Only show captcha required if not authenticated
	if sr.CaptchaRequired && sr.Status != "authenticated" {
		fmt.Println("  капча: требуется")
	}
}

func cmdCaptcha() {
	data, err := client.GetCaptcha()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	path := filepath.Join(configDir, "captcha.jpg")
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	fmt.Printf("  капча сохранена: %s\n", path)
	fmt.Println("  открой файл и введи код при логине")
}

func cmdDevice() {
	fmt.Println("  эмуляция проверки устройства...")

	nonce, err := client.GetDeviceNonce()
	if err != nil {
		fmt.Printf("  ошибка nonce: %v\n", err)
		return
	}
	fmt.Printf("  nonce получен: %s\n", nonce.RequestID)

	event := micb.SecurityEvent{}
	event.Accessibility.HasUntrusted = false
	event.Accessibility.Enabled = []string{}
	event.Accessibility.Allowlist = []string{}
	event.Overlay = false
	event.DeviceAdmin.HasOtherAdmins = false
	event.DeviceAdmin.ActiveAdminsCount = 0
	event.Root = false
	event.SignatureValid = true
	event.Debug.SDKInt = 0
	event.Debug.Manufacturer = "web"
	event.Debug.Model = "micb-go-cli"

	if err := client.SendSecurityEvent(event); err != nil {
		fmt.Printf("  ошибка security event: %v\n", err)
		return
	}
	fmt.Println("  ✓ проверка устройства пройдена")
}

func cmdLogin() {
	var login, password, captcha string
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("  Логин: ")
	login, _ = reader.ReadString('\n')
	login = strings.TrimSpace(login)

	fmt.Print("  Пароль: ")
	password, _ = reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// Auto-fetch captcha
	fmt.Println("  скачиваю капчу...")
	data, err := client.GetCaptcha()
	if err != nil {
		fmt.Printf("  ошибка капчи: %v\n", err)
		return
	}
	path := filepath.Join(configDir, "captcha.jpg")
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("  ошибка сохранения капчи: %v\n", err)
		return
	}
	fmt.Printf("  капча: %s\n", path)

	fmt.Print("  Код с капчи: ")
	captcha, _ = reader.ReadString('\n')
	captcha = strings.TrimSpace(captcha)

	client.SetLogin(login)

	sr, err := client.Login(micb.LoginRequest{
		LoginMethod: "LOGIN_PW",
		Login:       login,
		Password:    password,
		Captcha:     captcha,
	})
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}

	if sr.Status2 == "confirmationRequired" && sr.Confirmation != nil {
		fmt.Printf("  OTP отправлен на %s\n", sr.Confirmation.Challenge.Phone)
		fmt.Print("  Код из SMS: ")
		var otp string
		otp, _ = reader.ReadString('\n')
		otp = strings.TrimSpace(otp)

		sr, err = client.ConfirmOTP(micb.ConfirmOTPRequest{
			LoginMethod: "LOGIN_PW",
			Login:       login,
			Password:    password,
			Captcha:     captcha,
			Confirmation: micb.Confirmation{
				ConfirmationKey: sr.Confirmation.ConfirmationKey,
				AuthType:        sr.Confirmation.AuthType,
				Response:        otp,
			},
		})
		if err != nil {
			fmt.Printf("  ошибка OTP: %v\n", err)
			return
		}
	}

	if sr.User != nil {
		fmt.Printf("\n  ✓ %s\n", sr.User.DisplayName)
		fmt.Printf("  логин: %s\n", sr.User.Login)

		if err := client.CreateAPISession(); err != nil {
			fmt.Printf("  ошибка API-сессии: %v\n", err)
			return
		}

		if err := client.SaveCookies(cookieFile); err != nil {
			fmt.Printf("  ошибка сохранения: %v\n", err)
		}
	} else {
		fmt.Println("  не удалось войти")
		printJSON(sr)
	}
}

func cmdUser() {
	if !ensureAuth() {
		return
	}
	user, err := client.GetUser()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	fmt.Printf("  Имя:    %s\n", user.DisplayName)
	fmt.Printf("  Логин:  %s\n", user.Login)
	fmt.Printf("  ID:     %s\n", user.ID)
}

func cmdAccounts() {
	if !ensureAuth() {
		return
	}
	accounts, err := client.GetAccounts()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(accounts) == 0 {
		fmt.Println("  нет счетов")
		return
	}
	for _, a := range accounts {
		number := str(a, "number")
		balance := str(a, "balance")
		currency := str(a, "currency")
		name := str(a, "name")
		status := str(a, "status")

		icon := "●"
		if status != "active" && status != "" {
			icon = "○"
		}
		fmt.Printf("  %s %s  %s %s  %s\n", icon, number, balance, currency, name)
	}
}

func cmdCards() {
	if !ensureAuth() {
		return
	}
	cards, err := client.GetContracts()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(cards) == 0 {
		fmt.Println("  нет карт")
		return
	}
	for i, c := range cards {
		number := str(c, "number")
		balance := fmt.Sprintf("%.2f", num(c, "balances.available.value"))
		currency := str(c, "balances.available.currency")
		if currency == "" {
			currency = str(c, "currency")
		}
		status := str(c, "card.status")
		prodName := str(c, "product.name")
		ownerName := str(c, "owner.fullName")

		icon := "●"
		if status != "active" {
			icon = "○"
		}
		fmt.Printf("  [%d] %s %s  %s %s  %s  %s\n", i, icon, number, balance, currency, prodName, ownerName)
	}
}

func cmdCardInfo(args []string) {
	if !ensureAuth() {
		return
	}
	cards, err := client.GetContracts()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(cards) == 0 {
		fmt.Println("  нет карт")
		return
	}

	idx := 0
	if len(args) > 0 {
		idx, _ = strconv.Atoi(args[0])
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("  Номер карты (0-%d): ", len(cards)-1)
		idxStr, _ := reader.ReadString('\n')
		idxStr = strings.TrimSpace(idxStr)
		fmt.Sscanf(idxStr, "%d", &idx)
	}

	if idx < 0 || idx >= len(cards) {
		fmt.Println("  неверный номер")
		return
	}

	c := cards[idx]
	ccy := str(c, "balances.available.currency")
	if ccy == "" {
		ccy = str(c, "currency")
	}
	fmt.Printf("  Номер:       %s\n", str(c, "number"))
	fmt.Printf("  IBAN:        %s\n", str(c, "addData.EXT_IBAN"))
	fmt.Printf("  Баланс:      %.2f %s\n", num(c, "balances.available.value"), ccy)
	fmt.Printf("  Собств.ср-ва: %.2f %s\n", num(c, "balances.own_balance.value"), ccy)
	fmt.Printf("  Блокир:      %.2f %s\n", num(c, "balances.blocked.value"), ccy)
	fmt.Printf("  Кредит лимит: %.2f %s\n", num(c, "balances.cr_limit.value"), ccy)
	fmt.Printf("  Статус:      %s\n", str(c, "card.status"))
	fmt.Printf("  Тип:         %s\n", str(c, "addData.ACNT_CLASSIFIER_TYPE"))
	fmt.Printf("  Продукт:     %s\n", str(c, "product.name"))
	fmt.Printf("  Бренд:       %s\n", str(c, "product.brand"))
	fmt.Printf("  Срок:        %s\n", str(c, "card.expiryDate"))
	fmt.Printf("  Владелец:    %s\n", str(c, "owner.fullName"))
}

func cmdCardNumber(args []string) {
	if !ensureAuth() {
		return
	}
	cards, err := client.GetContracts()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(cards) == 0 {
		fmt.Println("  нет карт")
		return
	}
	idx := 0
	if len(args) > 0 {
		idx, _ = strconv.Atoi(args[0])
	} else {
		idx = 0
	}
	if idx < 0 || idx >= len(cards) {
		fmt.Printf("  неверный номер (0-%d)\n", len(cards)-1)
		return
	}
	c := cards[idx]

	// Номер карты может уже приходить в ответе (маскированный или полный)
	fullNumber := str(c, "number")
	if fullNumber != "" && !strings.Contains(fullNumber, "****") {
		// Полный номер уже есть
		fmt.Printf("  %s\n", fullNumber)
		return
	}

	// Если только маскированный, пробуем через card-req
	status := str(c, "card.status")
	if status != "active" && status != "ACTIVE" {
		fmt.Printf("  карта не активна (статус: %s)\n", status)
		return
	}
	cbs := str(c, "cbsNumber")
	if cbs == "" {
		fmt.Println("  нет cbsNumber для этой карты")
		return
	}
	fmt.Println("  запрашиваю номер карты...")
	number, err := client.GetCardNumber(cbs)
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	fmt.Printf("  %s\n", number)
}

func cmdNotifications() {
	if !ensureAuth() {
		return
	}
	items, err := client.GetNotifications()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(items) == 0 {
		fmt.Println("  нет уведомлений")
		return
	}
	for _, item := range items {
		title := str(item, "title")
		status := str(item, "status")
		date := str(item, "date")
		body := str(item, "body")
		icon := "●"
		if status == "read" {
			icon = "○"
		}
		fmt.Printf("  %s %s\n", icon, title)
		fmt.Printf("    %s\n", date)
		if len(body) > 0 {
			// Strip HTML tags for preview
			plain := stripHTML(body)
			if len(plain) > 120 {
				plain = plain[:120] + "..."
			}
			if plain != "" {
				fmt.Printf("    %s\n", plain)
			}
		}
		fmt.Println()
	}
}

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = html.UnescapeString(s)
	var inTag bool
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
			continue
		}
		if s[i] == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out = append(out, s[i])
		}
	}
	s2 := strings.Join(strings.Fields(string(out)), " ")
	return s2
}

func cmdDevices() {
	if !ensureAuth() {
		return
	}
	devices, err := client.GetMobileDevices()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(devices) == 0 {
		fmt.Println("  нет устройств")
		return
	}
	currentID := client.GetDeviceID()
	fmt.Println("  Устройства:")
	for i, d := range devices {
		marker := " "
		if d.ID == currentID {
			marker = "*"
		}
		fmt.Printf("  %s [%d] %s (%s)\n", marker, i, d.Name, d.ID)
	}
	fmt.Println("  * = текущий Device ID")
	fmt.Println("  Используй device-use <N> чтобы выбрать устройство")
}

func cmdDeviceUse(args []string) {
	if !ensureAuth() {
		return
	}
	devices, err := client.GetMobileDevices()
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	if len(args) == 0 {
		fmt.Println("  укажи номер устройства: device-use <N>")
		return
	}
	idx, _ := strconv.Atoi(args[0])
	if idx < 0 || idx >= len(devices) {
		fmt.Printf("  неверный номер (0-%d)\n", len(devices)-1)
		return
	}
	client.SetDeviceID(devices[idx].ID)
	fmt.Printf("  выбран Device ID: %s (%s)\n", devices[idx].ID, devices[idx].Name)
}

func cmdLogout() {
	if !client.IsAuthenticated() {
		fmt.Println("  не аутентифицирован")
		return
	}
	if err := client.Logout(cookieFile); err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}
	fmt.Println("  ✓ вышли из аккаунта")
}

func hasMMAProfile() bool {
	_, err := os.Stat(profileFile)
	return err == nil
}

func cmdMMASetup() {
	fmt.Println("  Настройка MMA (Mobile Multi-factor Authentication)")
	fmt.Println("  Требуется подтверждение через SMS")

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("  Device ID (или Enter для auto): ")
	deviceID, _ := reader.ReadString('\n')
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		deviceID = "cli-device-" + fmt.Sprintf("%x", sha1.Sum([]byte(time.Now().String())))[:16]
	}

	fmt.Print("  Label (имя устройства): ")
	label, _ := reader.ReadString('\n')
	label = strings.TrimSpace(label)
	if label == "" {
		label = "CLI"
	}

	fmt.Print("  Придумай PIN (6 цифр): ")
	pin, _ := reader.ReadString('\n')
	pin = strings.TrimSpace(pin)

	fmt.Print("  Твой логин: ")
	login, _ := reader.ReadString('\n')
	login = strings.TrimSpace(login)

	fmt.Println("  Отправляю запрос на сервер...")
	confirmationKey, phone, err := client.EnrollMMAStep1(deviceID, label, pin)
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}

	fmt.Printf("  SMS отправлен на %s\n", phone)
	fmt.Print("  Код из SMS: ")
	otp, _ := reader.ReadString('\n')
	otp = strings.TrimSpace(otp)

	profile, err := client.EnrollMMAStep2(deviceID, label, pin, otp, confirmationKey)
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}

	if profile.EncryptedKey == "" || profile.EncryptionParams.PBEParams.Salt == "" {
		fmt.Println("  ошибка: неполный профиль от сервера")
		return
	}

	profile.Login = login

	// Test the PIN by generating a password
	password, newTC, err := micb.GenerateSessionPassword(pin, *profile)
	if err != nil {
		fmt.Printf("  ошибка: неверный PIN или профиль: %v\n", err)
		return
	}

	// Update transaction counter
	profile.TransactionCounter = newTC

	if err := client.SaveProfile(profileFile, *profile); err != nil {
		fmt.Printf("  ошибка сохранения: %v\n", err)
		return
	}

	fmt.Printf("  ✓ MMA настроен! Сгенерирован пароль: %s\n", password)
	fmt.Println("  Теперь используй pin-login для входа")
}

func cmdPinLogin(args []string) {
	if !hasMMAProfile() {
		fmt.Println("  MMA не настроен. Сначала выполни mma-setup")
		return
	}

	profile, err := client.LoadProfile(profileFile)
	if err != nil {
		fmt.Printf("  ошибка загрузки профиля: %v\n", err)
		return
	}

	var pin string
	reader := bufio.NewReader(os.Stdin)
	if len(args) > 0 {
		pin = args[0]
	} else {
		fmt.Print("  PIN: ")
		pin, _ = reader.ReadString('\n')
		pin = strings.TrimSpace(pin)
	}

	password, newTC, err := micb.GenerateSessionPassword(pin, *profile)
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}

	// Update and save profile
	profile.TransactionCounter = newTC
	client.SaveProfile(profileFile, *profile)

	login := profile.Login
	if login == "" {
		fmt.Print("  Логин: ")
		login, _ = reader.ReadString('\n')
		login = strings.TrimSpace(login)
	}

	client.SetLogin(login)

	sr, err := client.LoginMMA(micb.LoginMMARequest{
		LoginMethod:   "LOGIN_MMA",
		Login:         login,
		Password:      password,
		AuthType:      "otp_mma",
		ApplicationID: profile.ID,
	})
	if err != nil {
		fmt.Printf("  ошибка: %v\n", err)
		return
	}

	if sr.Status2 == "confirmationRequired" && sr.Confirmation != nil {
		fmt.Printf("  OTP отправлен на %s\n", sr.Confirmation.Challenge.Phone)
		fmt.Print("  Код из SMS: ")
		var otp string
		otp, _ = reader.ReadString('\n')
		otp = strings.TrimSpace(otp)

		sr, err = client.ConfirmOTP(micb.ConfirmOTPRequest{
			LoginMethod: "LOGIN_MMA",
			Login:       login,
			Password:    password,
			Confirmation: micb.Confirmation{
				ConfirmationKey: sr.Confirmation.ConfirmationKey,
				AuthType:        sr.Confirmation.AuthType,
				Response:        otp,
			},
		})
		if err != nil {
			fmt.Printf("  ошибка OTP: %v\n", err)
			return
		}
	}

	if sr.User != nil {
		fmt.Printf("\n  ✓ %s\n", sr.User.DisplayName)

		client.SetMMACredentials(pin, profile)

		if err := client.CreateAPISession(); err != nil {
			fmt.Printf("  ошибка API-сессии: %v\n", err)
			return
		}

		if err := client.SaveCookies(cookieFile); err != nil {
			fmt.Printf("  ошибка сохранения: %v\n", err)
		}
	} else {
		fmt.Println("  не удалось войти")
		printJSON(sr)
	}
}

func ensureAuth() bool {
	if client.IsAuthenticated() {
		return true
	}
	if client.HasMMACredentials() {
		fmt.Print("  сессия истекла, обновляю...")
		if err := client.RefreshSession(); err != nil {
			fmt.Printf(" ошибка: %v\n", err)
		} else {
			client.SaveProfile(profileFile, *client.MMAProfile())
			fmt.Println(" ✓")
			return true
		}
	}
	fmt.Println("  не аутентифицирован. введи login")
	return false
}

func num(m map[string]interface{}, key string) float64 {
	parts := strings.Split(key, ".")
	cur := m
	for i, p := range parts {
		v, ok := cur[p]
		if !ok {
			return 0
		}
		if i == len(parts)-1 {
			if v == nil {
				return 0
			}
			switch n := v.(type) {
			case float64:
				return n
			case float32:
				return float64(n)
			case int:
				return float64(n)
			case int64:
				return float64(n)
			case json.Number:
				f, _ := n.Float64()
				return f
			case string:
				f, _ := strconv.ParseFloat(n, 64)
				return f
			default:
				f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
				return f
			}
		}
		if nm, ok := v.(map[string]interface{}); ok {
			cur = nm
		} else {
			return 0
		}
	}
	return 0
}

func str(m map[string]interface{}, key string) string {
	parts := strings.Split(key, ".")
	cur := m
	for i, p := range parts {
		v, ok := cur[p]
		if !ok {
			return ""
		}
		if i == len(parts)-1 {
			if v == nil {
				return ""
			}
			return fmt.Sprint(v)
		}
		if nm, ok := v.(map[string]interface{}); ok {
			cur = nm
		} else {
			return ""
		}
	}
	return ""
}

func printJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	fmt.Println(string(data))
}
