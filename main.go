package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// ---- Config ----

type Config struct {
	LoginURL  string
	UpdateURL string
	Username  string
	Password  string
	Interval  time.Duration
	Products  []Product
}

func loadConfig() *Config {
	// Load .env nếu có (local dev). Production/Render dùng env var thật → bỏ qua lỗi
	_ = godotenv.Load()

	cfg := &Config{}

	cfg.LoginURL = mustEnv("LOGIN_URL")
	cfg.UpdateURL = mustEnv("UPDATE_URL")
	cfg.Username = mustEnv("LOGIN_USERNAME")
	cfg.Password = mustEnv("LOGIN_PASSWORD")

	intervalStr := getEnvOrDefault("INTERVAL", "1h")
	d, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Fatalf("❌ INTERVAL không hợp lệ (%q): %v", intervalStr, err)
	}
	cfg.Interval = d

	productsJSON := mustEnv("PRODUCTS")
	if err := json.Unmarshal([]byte(productsJSON), &cfg.Products); err != nil {
		log.Fatalf("❌ PRODUCTS không parse được: %v", err)
	}
	if len(cfg.Products) == 0 {
		log.Fatal("❌ PRODUCTS không được để trống")
	}

	return cfg
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("❌ Env var %q chưa được set", key)
	}
	return v
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---- Models ----

type Product struct {
	Name            string `json:"name"`
	ShopAddressID   string `json:"shop_address_id"`
	WarehouseID     string `json:"warehouse_id"`
	ProductDetailID string `json:"product_detail_id"`
}

type LoginRequest struct {
	Input    string `json:"input"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type ModelItem struct {
	ShopAddressID   string `json:"shop_address_id"`
	WarehouseID     string `json:"warehouse_id"`
	ProductDetailID string `json:"product_detail_id"`
	Price           int    `json:"price"`
}

type UpdatePriceRequest struct {
	ModelList []ModelItem `json:"model_list"`
}

// ---- API ----

func login(cfg *Config) (string, error) {
	body, _ := json.Marshal(LoginRequest{Input: cfg.Username, Password: cfg.Password})

	req, err := http.NewRequest("POST", cfg.LoginURL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create login request: %w", err)
	}
	setCommonHeaders(req)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed (status %d): %s", resp.StatusCode, raw)
	}

	var lr LoginResponse
	if err := json.Unmarshal(raw, &lr); err != nil {
		return "", fmt.Errorf("parse login response: %w", err)
	}
	if lr.Data.AccessToken == "" {
		return "", fmt.Errorf("empty access token: %s", raw)
	}
	return lr.Data.AccessToken, nil
}

func updatePrice(cfg *Config, token string, p Product, price int) error {
	payload := UpdatePriceRequest{
		ModelList: []ModelItem{
			{
				ShopAddressID:   p.ShopAddressID,
				WarehouseID:     p.WarehouseID,
				ProductDetailID: p.ProductDetailID,
				Price:           price,
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("PUT", cfg.UpdateURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create update request: %w", err)
	}
	setCommonHeaders(req)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("x-language-key", "en")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("update request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update price failed (status %d): %s", resp.StatusCode, raw)
	}

	log.Printf("✅ [%s] price → %d | %s", p.Name, price, raw)
	return nil
}

func setCommonHeaders(req *http.Request) {
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
}

// ---- Job ----

func runJob(cfg *Config, roundIndex *int, prices []int) {
	idx := *roundIndex
	p := cfg.Products[idx]
	price := prices[idx]

	log.Printf("🔄 [%d/%d] %s — price: %d", idx+1, len(cfg.Products), p.Name, price)

	token, err := login(cfg)
	if err != nil {
		log.Printf("❌ Login error: %v", err)
		return
	}

	if err := updatePrice(cfg, token, p, price); err != nil {
		log.Printf("❌ Update error: %v", err)
		return
	}

	prices[idx]++
	*roundIndex = (idx + 1) % len(cfg.Products)
	log.Printf("➡️  Next: %s", cfg.Products[*roundIndex].Name)
}

// ---- Main ----

//func main() {
//	cfg := loadConfig()
//
//	prices := make([]int, len(cfg.Products))
//	for i := range prices {
//		prices[i] = 3001
//	}
//	roundIndex := 0
//
//	log.Printf("🚀 Started — user: %s | %d products | interval: %s",
//		cfg.Username, len(cfg.Products), cfg.Interval)
//
//	runJob(cfg, &roundIndex, prices)
//
//	ticker := time.NewTicker(cfg.Interval)
//	defer ticker.Stop()
//
//	for range ticker.C {
//		runJob(cfg, &roundIndex, prices)
//	}
//}

// ---- Main ----

func main() {
	cfg := loadConfig()

	prices := make([]int, len(cfg.Products))
	for i := range prices {
		prices[i] = 3001
	}
	roundIndex := 0

	log.Printf("🚀 Started — user: %s | %d products | Đã chuyển giao lịch chạy cho cron-job.org", cfg.Username, len(cfg.Products))

	// 1. Mỗi lần thằng cron-job.org gõ cửa (ping), hàm này sẽ kích hoạt NGAY LẬP TỨC
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		// Chạy cái job quét sản phẩm của bạn
		runJob(cfg, &roundIndex, prices)

		// Trả lời cho cron-job.org biết là đã chạy xong xuôi
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Job executed successfully at: %s", time.Now().String())
	})

	// 2. Mở cổng mạng để Render cấp link
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🌐 HTTP Server đang chờ cron-job.org tại cổng: %s", port)

	// 3. Treo server ở đây để chờ nhận ping
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("❌ Lỗi Server: %v", err)
	}
}
