package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	chart "github.com/wcharczuk/go-chart/v2"
)

// KlineResponse - структура API відповіді для свічок
type KlineResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List [][]interface{} `json:"list"`
	} `json:"result"`
}

func tgHandleKline(symbol string) string {
	if symbol == "" {
		return "Вкажіть символ, наприклад: /kline BTCUSDT"
	}
	symbol = strings.ToUpper(symbol)
	// Validate symbol against supported tickers
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання списку символів: " + err.Error()
	}
	supported := false
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Sprintf("Символ %s не підтримується Bybit.", symbol)
	}
	url := fmt.Sprintf("https://api.bybit.com/v5/market/kline?category=spot&symbol=%s&interval=1&limit=5", symbol)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "Помилка створення запиту: " + err.Error()
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Referer", "https://bybit.com/")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Помилка HTTP: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Помилка читання: " + err.Error()
	}
	log.Printf("Bybit kline response for %s: %s", symbol, string(body))
	var kline KlineResponse
	if err = json.Unmarshal(body, &kline); err != nil {
		return "Помилка JSON: " + err.Error()
	}
	if kline.RetCode != 0 {
		return "Помилка API: " + kline.RetMsg
	}
	if len(kline.Result.List) == 0 {
		return "Немає даних для " + symbol
	}
	// Візуалізація: графік закриття
	closes := make([]float64, 0, len(kline.Result.List))
	for _, item := range kline.Result.List {
		if len(item) < 5 {
			continue
		}
		closeVal, ok := item[4].(string)
		if !ok {
			continue
		}
		f, _ := parseFloat(closeVal)
		closes = append(closes, f)
	}
	maxClose := 0.0
	for _, v := range closes {
		if v > maxClose {
			maxClose = v
		}
	}
	res := fmt.Sprintf("Останні 5 свічок %s (закриття):\n", symbol)
	for i, v := range closes {
		barLen := 0
		if maxClose > 0 {
			barLen = int((v / maxClose) * 20)
		}
		bar := strings.Repeat("█", barLen)
		res += fmt.Sprintf("`%d: %8.2f %s`\n", i+1, v, bar)
	}
	return res
}

func tgHandleKlinePhoto(symbolsRaw string, bot *tgbotapi.BotAPI, chatID int64) string {
	symbols := strings.Split(symbolsRaw, ",")
	if len(symbols) == 0 || (len(symbols) == 1 && strings.TrimSpace(symbols[0]) == "") {
		return "Вкажіть символи через кому, наприклад: /klinephoto BTCUSDT,ETHUSDT"
	}
	// Fetch supported tickers once
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання списку символів: " + err.Error()
	}
	// colors will be assigned using chart.GetDefaultColor
	series := []chart.Series{}
	legend := []string{}
	maxLen := 0
	var errors []string
	for i, symbol := range symbols {
		symbol = strings.TrimSpace(strings.ToUpper(symbol))
		if symbol == "" {
			continue
		}
		// Validate symbol
		supported := false
		for _, t := range tickers.Result.List {
			if t.Symbol == symbol {
				supported = true
				break
			}
		}
		if !supported {
			errors = append(errors, fmt.Sprintf("%s: Символ не підтримується Bybit", symbol))
			continue
		}
		url := fmt.Sprintf("https://api.bybit.com/v5/market/kline?category=spot&symbol=%s&interval=1&limit=20", symbol)
		log.Printf("Requesting URL: %s", url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Printf("Failed to create request for %s: %v", symbol, err)
			errors = append(errors, fmt.Sprintf("%s: Помилка створення запиту", symbol))
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("HTTP error for %s: %v", symbol, err)
			errors = append(errors, fmt.Sprintf("%s: Помилка HTTP", symbol))
			continue
		}
		log.Printf("HTTP status for %s: %d %s", symbol, resp.StatusCode, resp.Status)
		log.Printf("HTTP headers for %s: %v", symbol, resp.Header)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Read error for %s: %v", symbol, err)
			errors = append(errors, fmt.Sprintf("%s: Помилка читання", symbol))
			continue
		}
		log.Printf("Bybit kline response for %s: %s", symbol, string(body))
		var kline KlineResponse
		if err = json.Unmarshal(body, &kline); err != nil {
			log.Printf("JSON error for %s: %v", symbol, err)
			errors = append(errors, fmt.Sprintf("%s: Помилка JSON", symbol))
			continue
		}
		if kline.RetCode != 0 || len(kline.Result.List) == 0 {
			log.Printf("API error for %s: %s", symbol, kline.RetMsg)
			errors = append(errors, fmt.Sprintf("%s: Помилка API: %s", symbol, kline.RetMsg))
			continue
		}
		closes := make([]float64, 0, len(kline.Result.List))
		for _, item := range kline.Result.List {
			if len(item) < 5 {
				continue
			}
			closeVal, ok := item[4].(string)
			if !ok {
				continue
			}
			f, _ := parseFloat(closeVal)
			closes = append(closes, f)
		}
		if len(closes) < 2 {
			errors = append(errors, fmt.Sprintf("%s: Недостатньо даних для розрахунку змін", symbol))
			continue
		}
		changes := make([]float64, len(closes)-1)
		for j := 1; j < len(closes); j++ {
			changes[j-1] = closes[j] - closes[j-1]
		}
		if len(changes) > maxLen {
			maxLen = len(changes)
		}
		xValues := make([]float64, len(changes))
		for j := range changes {
			xValues[j] = float64(j + 1)
		}
		color := chart.GetDefaultColor(i)
		series = append(series, chart.ContinuousSeries{
			Name:    symbol,
			XValues: xValues,
			YValues: changes,
			Style: chart.Style{
				StrokeWidth: 3.0,
				StrokeColor: color,
			},
		})
		legend = append(legend, symbol)
	}
	if len(series) == 0 {
		return "Немає даних для заданих символів.\n" + strings.Join(errors, "\n")
	}
	graph := chart.Chart{
		Width:      600,
		Height:     300,
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 40, Right: 20, Bottom: 20}},
		Series:     series,
		YAxis:      chart.YAxis{},
		XAxis:      chart.XAxis{},
		Elements: []chart.Renderable{
			chart.Legend(&chart.Chart{
				Series: series,
			}),
		},
	}
	buf := bytes.NewBuffer([]byte{})
	if err = graph.Render(chart.PNG, buf); err != nil {
		return "Помилка рендеру графіка: " + err.Error()
	}
	photoFileBytes := tgbotapi.FileBytes{Name: "kline_compare.png", Bytes: buf.Bytes()}
	photoMsg := tgbotapi.NewPhoto(chatID, photoFileBytes)
	photoMsg.Caption = "Порівняння годинних змін: " + strings.Join(legend, ", ")
	_, err = bot.Send(photoMsg)
	if err != nil {
		return "Помилка надсилання фото: " + err.Error()
	}
	if len(errors) > 0 {
		return "Графік порівняння надіслано!\n" + strings.Join(errors, "\n")
	}
	return "Графік порівняння надіслано!"
}

func tgHandleVolumePhoto(bot *tgbotapi.BotAPI, chatID int64) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Error fetching volume: " + err.Error()
	}
	type pair struct {
		Symbol string
		Volume float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		v, err := parseFloat(t.Volume24h)
		if err != nil {
			continue
		}
		pairs = append(pairs, pair{t.Symbol, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Volume > pairs[j].Volume })
	if len(pairs) == 0 {
		return "Немає даних для об'єму."
	}
	max := 5
	if len(pairs) < 5 {
		max = len(pairs)
	}
	labels := make([]string, max)
	values := make([]float64, max)
	for i := 0; i < max; i++ {
		labels[i] = pairs[i].Symbol
		values[i] = pairs[i].Volume
	}
	bar := chart.BarChart{
		Width:      600,
		Height:     300,
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 40, Right: 20, Bottom: 20}},
		Bars:       []chart.Value{},
	}
	for i := 0; i < max; i++ {
		bar.Bars = append(bar.Bars, chart.Value{Value: values[i], Label: labels[i]})
	}
	buf := bytes.NewBuffer([]byte{})
	if err := bar.Render(chart.PNG, buf); err != nil {
		return "Помилка рендеру графіка: " + err.Error()
	}
	photoFileBytes := tgbotapi.FileBytes{Name: "volume_bar.png", Bytes: buf.Bytes()}
	photoMsg := tgbotapi.NewPhoto(chatID, photoFileBytes)
	photoMsg.Caption = "Топ-5 монет за об'ємом"
	_, err = bot.Send(photoMsg)
	if err != nil {
		return "Помилка надсилання фото: " + err.Error()
	}
	return "Графік об'єму надіслано!"
}

func tgHandleSalesPhoto(bot *tgbotapi.BotAPI, chatID int64) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Error fetching sales: " + err.Error()
	}
	type pair struct {
		Symbol string
		Sales  float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		v, err := parseFloat(t.Volume24h)
		if err != nil {
			continue
		}
		pairs = append(pairs, pair{t.Symbol, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Sales > pairs[j].Sales })
	if len(pairs) == 0 {
		return "Немає даних для продажу."
	}
	max := 5
	if len(pairs) < 5 {
		max = len(pairs)
	}
	labels := make([]string, max)
	values := make([]float64, max)
	for i := 0; i < max; i++ {
		labels[i] = pairs[i].Symbol
		values[i] = pairs[i].Sales
	}
	bar := chart.BarChart{
		Width:      600,
		Height:     300,
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 40, Right: 20, Bottom: 20}},
		Bars:       []chart.Value{},
	}
	for i := 0; i < max; i++ {
		bar.Bars = append(bar.Bars, chart.Value{Value: values[i], Label: labels[i]})
	}
	buf := bytes.NewBuffer([]byte{})
	if err := bar.Render(chart.PNG, buf); err != nil {
		return "Помилка рендеру графіка: " + err.Error()
	}
	photoFileBytes := tgbotapi.FileBytes{Name: "sales_bar.png", Bytes: buf.Bytes()}
	photoMsg := tgbotapi.NewPhoto(chatID, photoFileBytes)
	photoMsg.Caption = "Топ-5 монет за об'ємом продажу"
	_, err = bot.Send(photoMsg)
	if err != nil {
		return "Помилка надсилання фото: " + err.Error()
	}
	return "Графік продажу надіслано!"
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func tgHandlePrice(symbol string) string {
	if symbol == "" {
		return "Вкажіть символ, наприклад: /price BTCUSDT"
	}
	symbol = strings.ToUpper(symbol)
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання ціни: " + err.Error()
	}
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			return fmt.Sprintf("%s ціна: %s", symbol, t.LastPrice)
		}
	}
	return "Символ не знайдено."
}

func tgHandleChange(symbol string) string {
	if symbol == "" {
		return "Вкажіть символ, наприклад: /change BTCUSDT"
	}
	symbol = strings.ToUpper(symbol)
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання даних: " + err.Error()
	}
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			return fmt.Sprintf("%s зміна за 24г: %s%%", symbol, t.Price24hPcnt)
		}
	}
	return "Символ не знайдено."
}

func tgHandleVolume() string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання об'єму: " + err.Error()
	}
	type pair struct {
		Symbol string
		Volume float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		v, _ := parseFloat(t.Volume24h)
		pairs = append(pairs, pair{t.Symbol, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Volume > pairs[j].Volume })
	res := "Топ-5 за об'ємом:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.0f\n", pairs[i].Symbol, pairs[i].Volume)
	}
	return res
}

func tgHandleGainers() string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання лідерів: " + err.Error()
	}
	type pair struct {
		Symbol string
		Change float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		c, _ := parseFloat(t.Price24hPcnt)
		pairs = append(pairs, pair{t.Symbol, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Change > pairs[j].Change })
	res := "Топ-5 лідерів:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
	return res
}

func tgHandleLosers() string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання аутсайдерів: " + err.Error()
	}
	type pair struct {
		Symbol string
		Change float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		c, _ := parseFloat(t.Price24hPcnt)
		pairs = append(pairs, pair{t.Symbol, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Change < pairs[j].Change })
	res := "Топ-5 аутсайдерів:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
	return res
}

// FearGreedResponse represents the Alternative.me API response for Fear & Greed Index
type FearGreedResponse struct {
	Name string `json:"name"`
	Data []struct {
		Value               string `json:"value"`
		ValueClassification string `json:"value_classification"`
		Timestamp           string `json:"timestamp"`
	} `json:"data"`
}

func tgHandleGreed() string {
	url := "https://api.alternative.me/fng/?limit=1"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "Помилка створення запиту: " + err.Error()
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Помилка HTTP: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Помилка читання: " + err.Error()
	}
	var fg FearGreedResponse
	if err = json.Unmarshal(body, &fg); err != nil {
		return "Помилка JSON: " + err.Error()
	}
	if len(fg.Data) == 0 {
		return "Немає даних для індексу Страху та Жадібності."
	}
	value := fg.Data[0].Value
	classification := fg.Data[0].ValueClassification
	// Translate classification to Ukrainian
	switch classification {
	case "Extreme Fear":
		classification = "Екстремальний страх"
	case "Fear":
		classification = "Страх"
	case "Neutral":
		classification = "Нейтрально"
	case "Greed":
		classification = "Жадібність"
	case "Extreme Greed":
		classification = "Екстремальна жадібність"
	}
	timestamp := fg.Data[0].Timestamp
	ts, _ := parseFloat(timestamp)
	location, _ := time.LoadLocation("Europe/Kiev")
	t := time.Unix(int64(ts), 0).In(location)
	dateStr := t.Format("02-01-2006")
	timeStr := t.Format("15:04:05")
	return fmt.Sprintf("Індекс Страху та Жадібності: %s (%s)\nОновлено: %s\nЧас оновлення: %s", value, classification, dateStr, timeStr)
}

func checkSingleInstance() {
	pidFile := "bot.pid"
	if _, err := os.Stat(pidFile); err == nil {
		// PID file exists, check if process is running
		data, err := os.ReadFile(pidFile)
		if err == nil {
			var existingPid int
			if _, err := fmt.Sscanf(string(data), "%d", &existingPid); err == nil {
				// Check if process is running
				process, err := os.FindProcess(existingPid)
				if err == nil {
					// Try to send signal 0 to check if process exists
					err = process.Signal(syscall.Signal(0))
					if err == nil {
						log.Printf("Another instance is running (PID: %d). Exiting.", existingPid)
						os.Exit(1)
					}
				}
			}
		}
		// Remove stale PID file
		os.Remove(pidFile)
	}

	// Write current PID to file
	pid := os.Getpid()
	file, err := os.Create(pidFile)
	if err != nil {
		log.Printf("Warning: Could not create PID file: %v", err)
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%d\n", pid)

	// Handle cleanup on exit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Remove(pidFile)
		os.Exit(0)
	}()
}

func main() {
	log.SetOutput(os.Stdout)
	// Завантажуємо змінні середовища з .env (якщо файл існує)
	_ = godotenv.Load()
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		log.Panic("TELEGRAM_TOKEN не встановлено. Встановіть змінну середовища або додайте її в .env")
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		log.Panic("WEBHOOK_URL не встановлено. Встановіть змінну середовища або додайте її в .env")
	}

	webhook, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		log.Panic(err)
	}
	_, err = bot.Request(webhook)
	if err != nil {
		log.Panic(err)
	}

	info, err := bot.GetWebhookInfo()
	if err != nil {
		log.Panic(err)
	}
	if info.LastErrorDate != 0 {
		log.Printf("Telegram callback failed: %s", info.LastErrorMessage)
	}

	updates := bot.ListenForWebhook("/webhook")
	go http.ListenAndServe(":8080", nil)

	log.Println("Bot is running with webhook")

	alerts := make(map[string]float64)
	go func() {
		for {
			time.Sleep(30 * time.Second)
			if len(alerts) == 0 {
				continue
			}
			tickers, err := fetchTickers()
			if err != nil {
				log.Printf("Alert fetch error: %v", err)
				continue
			}
			for symbol, target := range alerts {
				for _, t := range tickers.Result.List {
					if t.Symbol == symbol {
						price, _ := parseFloat(t.LastPrice)
						if price >= target {
							log.Printf("ALERT: %s price %.2f >= %.2f", symbol, price, target)
							delete(alerts, symbol)
						}
					}
				}
			}
		}
	}()

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := update.Message.Text
		chatID := update.Message.Chat.ID

		if text == "/start" {
			keyboard := tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("BTCUSDT"),
					tgbotapi.NewKeyboardButton("/change BTCUSDT"),
					tgbotapi.NewKeyboardButton("/kline BTCUSDT"),
				),
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("/volume"),
					tgbotapi.NewKeyboardButton("/gainers"),
					tgbotapi.NewKeyboardButton("/losers"),
					tgbotapi.NewKeyboardButton("/greed"),
				),
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("/klinephoto BTCUSDT,ETHUSDT"),
					tgbotapi.NewKeyboardButton("/salesphoto"),
				),
			)
			//msg := tgbotapi.NewMessage(chatID, "Вітаю! Виберіть команду або введіть свою:")
			//
			commandsDescription := `👋 Вітаю!
			Оберіть команду нижче або введіть свою:

			💰 Ціни та зміни
			/price - поточна ціна
			/change — зміна за 24 год.

			📊 Ринок
			/volume — топ-5 пар за обсягом
			/gainers — топ-5 лідерів зростання
			/losers — топ-5 лідерів падіння
			/greed — індекс Страху та Жадібності

			📈 Графіки та свічки
			/kline — останні 5 свічок BTC/USDT
			/klinephoto — графік для кількох пар
			/volumephoto — графік топ-5 за обсягом
			/salesphoto — графік топ-5 за продажами
			`
			msg := tgbotapi.NewMessage(chatID, commandsDescription)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
			continue
		}

		if text == "BTCUSDT" {
			msg := tgbotapi.NewMessage(chatID, tgHandlePrice("BTCUSDT"))
			bot.Send(msg)
			continue
		}

		if strings.HasPrefix(text, "/price") {
			symbol := strings.TrimSpace(strings.TrimPrefix(text, "/price"))
			msg := tgbotapi.NewMessage(chatID, tgHandlePrice(symbol))
			bot.Send(msg)
			continue
		}
		if strings.HasPrefix(text, "/change") {
			symbol := strings.TrimSpace(strings.TrimPrefix(text, "/change"))
			msg := tgbotapi.NewMessage(chatID, tgHandleChange(symbol))
			bot.Send(msg)
			continue
		}
		if text == "/volume" {
			msg := tgbotapi.NewMessage(chatID, tgHandleVolume())
			bot.Send(msg)
			continue
		}
		if text == "/gainers" {
			msg := tgbotapi.NewMessage(chatID, tgHandleGainers())
			bot.Send(msg)
			continue
		}
		if text == "/losers" {
			msg := tgbotapi.NewMessage(chatID, tgHandleLosers())
			bot.Send(msg)
			continue
		}
		if text == "/greed" {
			msg := tgbotapi.NewMessage(chatID, tgHandleGreed())
			bot.Send(msg)
			continue
		}
		if strings.HasPrefix(text, "/klinephoto") {
			symbols := strings.TrimSpace(strings.TrimPrefix(text, "/klinephoto"))
			msg := tgHandleKlinePhoto(symbols, bot, chatID)
			bot.Send(tgbotapi.NewMessage(chatID, msg))
			continue
		}
		if strings.HasPrefix(text, "/kline") {
			symbol := strings.TrimSpace(strings.TrimPrefix(text, "/kline"))
			msg := tgbotapi.NewMessage(chatID, tgHandleKline(symbol))
			bot.Send(msg)
			continue
		}
		if text == "/volumephoto" {
			msg := tgHandleVolumePhoto(bot, chatID)
			bot.Send(tgbotapi.NewMessage(chatID, msg))
			continue
		}
		if text == "/salesphoto" {
			msg := tgHandleSalesPhoto(bot, chatID)
			bot.Send(tgbotapi.NewMessage(chatID, msg))
			continue
		}
		msg := tgbotapi.NewMessage(chatID, "Невідома команда.")
		bot.Send(msg)
	}
}
