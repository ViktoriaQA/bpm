package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	log.SetOutput(os.Stdout)
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_TOKEN"))
	if err != nil {
		log.Panic(err)
	}
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	alerts := make(map[string]float64)
	go alertWorker(alerts)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := update.Message.Text
		chatID := update.Message.Chat.ID

		if text == "/start" {
			keyboard := tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("/price BTCUSDT"),
					tgbotapi.NewKeyboardButton("/change BTCUSDT"),
				),
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("/volume"),
					tgbotapi.NewKeyboardButton("/gainers"),
					tgbotapi.NewKeyboardButton("/losers"),
				),
			)
			msg := tgbotapi.NewMessage(chatID, "Вітаю! Виберіть команду або введіть свою:")
			msg.ReplyMarkup = keyboard
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
		msg := tgbotapi.NewMessage(chatID, "Невідома команда.")
		bot.Send(msg)
	}
}

// Telegram handlers (return string for bot)
func tgHandlePrice(symbol string) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Error fetching price: " + err.Error()
	}
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			return fmt.Sprintf("%s price: %s", symbol, t.LastPrice)
		}
	}
	return "Symbol not found."
}

func tgHandleChange(symbol string) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Error fetching change: " + err.Error()
	}
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			return fmt.Sprintf("%s 24h change: %s%%", symbol, t.Price24hPcnt)
		}
	}
	return "Symbol not found."
}

func tgHandleVolume() string {
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
		v, _ := parseFloat(t.Volume24h)
		pairs = append(pairs, pair{t.Symbol, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Volume > pairs[j].Volume })
	res := "Top 5 by volume:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f\n", pairs[i].Symbol, pairs[i].Volume)
	}
	return res
}

func tgHandleGainers() string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Error fetching gainers: " + err.Error()
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
	res := "Top 5 gainers:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
	return res
}

func tgHandleLosers() string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Error fetching losers: " + err.Error()
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
	res := "Top 5 losers:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
	return res
}

func alertWorker(alerts map[string]float64) {
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
						fmt.Printf("ALERT: %s price %.2f >= %.2f\n", symbol, price, target)
						delete(alerts, symbol)
					}
				}
			}
		}
	}
}

func handlePrice(symbol string) {
	tickers, err := fetchTickers()
	if err != nil {
		log.Printf("Error fetching price for %s: %v", symbol, err)
		fmt.Println("Error fetching price:", err)
		return
	}
	found := false
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			fmt.Printf("%s price: %s\n", symbol, t.LastPrice)
			found = true
			break
		}
	}
	if !found {
		log.Printf("Symbol not found: %s", symbol)
		fmt.Println("Symbol not found.")
	}
}

func handleChange(symbol string) {
	tickers, err := fetchTickers()
	if err != nil {
		log.Printf("Error fetching change for %s: %v", symbol, err)
		fmt.Println("Error fetching change:", err)
		return
	}
	found := false
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			fmt.Printf("%s 24h change: %s%%\n", symbol, t.Price24hPcnt)
			found = true
			break
		}
	}
	if !found {
		log.Printf("Symbol not found: %s", symbol)
		fmt.Println("Symbol not found.")
	}
}

func handleVolume() {
	tickers, err := fetchTickers()
	if err != nil {
		log.Printf("Error fetching volume: %v", err)
		fmt.Println("Error fetching volume:", err)
		return
	}
	type pair struct {
		Symbol string
		Volume float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		v, err := parseFloat(t.Volume24h)
		if err != nil {
			log.Printf("Error parsing volume for %s: %v", t.Symbol, err)
			continue
		}
		pairs = append(pairs, pair{t.Symbol, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Volume > pairs[j].Volume })
	fmt.Println("Top 5 by volume:")
	for i := 0; i < 5 && i < len(pairs); i++ {
		fmt.Printf("%s: %.2f\n", pairs[i].Symbol, pairs[i].Volume)
	}
}

func handleGainers() {
	tickers, err := fetchTickers()
	if err != nil {
		log.Printf("Error fetching gainers: %v", err)
		fmt.Println("Error fetching gainers:", err)
		return
	}
	type pair struct {
		Symbol string
		Change float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		c, err := parseFloat(t.Price24hPcnt)
		if err != nil {
			log.Printf("Error parsing change for %s: %v", t.Symbol, err)
			continue
		}
		pairs = append(pairs, pair{t.Symbol, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Change > pairs[j].Change })
	fmt.Println("Top 5 gainers:")
	for i := 0; i < 5 && i < len(pairs); i++ {
		fmt.Printf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
}

func handleLosers() {
	tickers, err := fetchTickers()
	if err != nil {
		log.Printf("Error fetching losers: %v", err)
		fmt.Println("Error fetching losers:", err)
		return
	}
	type pair struct {
		Symbol string
		Change float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		c, err := parseFloat(t.Price24hPcnt)
		if err != nil {
			log.Printf("Error parsing change for %s: %v", t.Symbol, err)
			continue
		}
		pairs = append(pairs, pair{t.Symbol, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Change < pairs[j].Change })
	fmt.Println("Top 5 losers:")
	for i := 0; i < 5 && i < len(pairs); i++ {
		fmt.Printf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
