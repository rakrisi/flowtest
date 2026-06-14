package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

// Test credentials for auth endpoints
const (
	testBearerToken = "test-bearer-token-12345"
	testBasicUser   = "testuser"
	testBasicPass   = "testpassword"
	testAPIKey      = "test-api-key-67890"
)

var (
	db          *pgxpool.Pool
	rdb         *redis.Client
	kafkaWriter *kafka.Writer
)

func main() {
	dbHost := envOr("DB_HOST", "localhost")
	dbPort := envOr("DB_PORT", "5432")
	dbUser := envOr("DB_USER", "myuser")
	dbPass := envOr("DB_PASSWORD", "mypassword")
	dbName := envOr("DB_NAME", "referral")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	kafkaBroker := envOr("KAFKA_BROKER", "localhost:9092")
	apiPort := envOr("API_PORT", "8000")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)

	// Connect to Postgres
	var err error
	db, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping postgres: %v", err)
	}
	log.Println("Connected to Postgres")

	// Connect to Redis
	rdb = redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to redis: %v", err)
	}
	defer rdb.Close()
	log.Println("Connected to Redis")

	// Kafka writer
	kafkaWriter = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaBroker),
		Balancer:               &kafka.LeastBytes{},
		WriteTimeout:           10 * time.Second,
		AllowAutoTopicCreation: true,
	}
	defer kafkaWriter.Close()
	log.Println("Kafka writer ready")

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /items", handleListItems)
	mux.HandleFunc("GET /items/{id}", handleGetItem)
	mux.HandleFunc("POST /items", handleCreateItem)
	mux.HandleFunc("PUT /items/{id}", handleUpdateItem)
	mux.HandleFunc("PATCH /items/{id}", handlePatchItem)
	mux.HandleFunc("DELETE /items/{id}", handleDeleteItem)

	// Auth test endpoints
	mux.HandleFunc("GET /auth/bearer", handleBearerAuth)
	mux.HandleFunc("GET /auth/basic", handleBasicAuth)
	mux.HandleFunc("GET /auth/api-key", handleAPIKeyAuth)
	mux.HandleFunc("POST /auth/bearer", handleBearerAuth)

	// Echo endpoint for testing advanced body/headers
	mux.HandleFunc("POST /echo", handleEcho)
	mux.HandleFunc("PUT /echo", handleEcho)
	mux.HandleFunc("PATCH /echo", handleEcho)
	mux.HandleFunc("POST /auth/basic", handleBasicAuth)
	mux.HandleFunc("POST /auth/api-key", handleAPIKeyAuth)

	log.Printf("Test API server listening on :%s", apiPort)
	log.Fatal(http.ListenAndServe(":"+apiPort, mux))
}

// --- Handlers ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

func handleListItems(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(), "SELECT id, name, description, price, status, created_at, updated_at FROM items ORDER BY id")
	if err != nil {
		writeJSON(w, 500, map[string]interface{}{"error": err.Error()})
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id int
		var name, description, status string
		var price float64
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &description, &price, &status, &createdAt, &updatedAt); err != nil {
			writeJSON(w, 500, map[string]interface{}{"error": err.Error()})
			return
		}
		items = append(items, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"price":       price,
			"status":      status,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		})
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	writeJSON(w, 200, map[string]interface{}{"items": items, "total": len(items)})
}

func handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid id"})
		return
	}

	var name, description, status string
	var price float64
	var createdAt, updatedAt time.Time
	err = db.QueryRow(r.Context(),
		"SELECT name, description, price, status, created_at, updated_at FROM items WHERE id = $1", id,
	).Scan(&name, &description, &price, &status, &createdAt, &updatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]interface{}{"error": "item not found"})
		return
	}

	item := map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
		"price":       price,
		"status":      status,
		"created_at":  createdAt.Format(time.RFC3339),
		"updated_at":  updatedAt.Format(time.RFC3339),
	}

	// Cache in Redis (JSON, 5 min TTL)
	cacheKey := fmt.Sprintf("item:%d", id)
	data, _ := json.Marshal(item)
	rdb.Set(r.Context(), cacheKey, string(data), 5*time.Minute)

	writeJSON(w, 200, item)
}

func handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid JSON body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, 400, map[string]interface{}{"error": "name is required"})
		return
	}

	var id int
	var createdAt, updatedAt time.Time
	err := db.QueryRow(r.Context(),
		"INSERT INTO items (name, description, price) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at",
		body.Name, body.Description, body.Price,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]interface{}{"error": err.Error()})
		return
	}

	item := map[string]interface{}{
		"id":          id,
		"name":        body.Name,
		"description": body.Description,
		"price":       body.Price,
		"status":      "active",
		"created_at":  createdAt.Format(time.RFC3339),
		"updated_at":  updatedAt.Format(time.RFC3339),
	}

	// Publish Kafka event
	go publishEvent("item.created", item)

	// Cache in Redis
	cacheKey := fmt.Sprintf("item:%d", id)
	data, _ := json.Marshal(item)
	rdb.Set(r.Context(), cacheKey, string(data), 5*time.Minute)

	writeJSON(w, 201, item)
}

func handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid id"})
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
		Status      string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid JSON body"})
		return
	}

	var updatedAt time.Time
	err = db.QueryRow(r.Context(),
		`UPDATE items SET name = $1, description = $2, price = $3, status = $4, updated_at = NOW()
		 WHERE id = $5 RETURNING updated_at`,
		body.Name, body.Description, body.Price, body.Status, id,
	).Scan(&updatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]interface{}{"error": "item not found"})
		return
	}

	item := map[string]interface{}{
		"id":          id,
		"name":        body.Name,
		"description": body.Description,
		"price":       body.Price,
		"status":      body.Status,
		"updated_at":  updatedAt.Format(time.RFC3339),
	}

	// Update Redis cache
	cacheKey := fmt.Sprintf("item:%d", id)
	data, _ := json.Marshal(item)
	rdb.Set(r.Context(), cacheKey, string(data), 5*time.Minute)

	// Publish Kafka event
	go publishEvent("item.updated", item)

	writeJSON(w, 200, item)
}

func handlePatchItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid id"})
		return
	}

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid JSON body"})
		return
	}

	// Build dynamic UPDATE
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	for _, field := range []string{"name", "description", "price", "status"} {
		if val, ok := body[field]; ok {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argIdx))
			args = append(args, val)
			argIdx++
		}
	}

	if len(setClauses) == 0 {
		writeJSON(w, 400, map[string]interface{}{"error": "no fields to update"})
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE items SET %s WHERE id = $%d RETURNING name, description, price, status, updated_at",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	var name, description, status string
	var price float64
	var updatedAt time.Time
	err = db.QueryRow(r.Context(), query, args...).Scan(&name, &description, &price, &status, &updatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]interface{}{"error": "item not found"})
		return
	}

	item := map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
		"price":       price,
		"status":      status,
		"updated_at":  updatedAt.Format(time.RFC3339),
	}

	// Update Redis cache
	cacheKey := fmt.Sprintf("item:%d", id)
	data, _ := json.Marshal(item)
	rdb.Set(r.Context(), cacheKey, string(data), 5*time.Minute)

	// Publish Kafka event
	go publishEvent("item.patched", item)

	writeJSON(w, 200, item)
}

func handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, 400, map[string]interface{}{"error": "invalid id"})
		return
	}

	tag, err := db.Exec(r.Context(), "DELETE FROM items WHERE id = $1", id)
	if err != nil {
		writeJSON(w, 500, map[string]interface{}{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]interface{}{"error": "item not found"})
		return
	}

	// Remove from Redis
	cacheKey := fmt.Sprintf("item:%d", id)
	rdb.Del(r.Context(), cacheKey)

	// Publish Kafka event
	go publishEvent("item.deleted", map[string]interface{}{"id": id})

	writeJSON(w, 200, map[string]interface{}{"deleted": true, "id": id})
}

// --- Auth Handlers ---

func handleBearerAuth(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		writeJSON(w, 401, map[string]interface{}{"error": "missing Authorization header"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid Authorization header format, expected 'Bearer <token>'"})
		return
	}

	token := parts[1]
	if token != testBearerToken {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid bearer token"})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"authenticated": true,
		"auth_type":     "bearer",
		"message":       "Bearer token authentication successful",
	})
}

func handleBasicAuth(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		writeJSON(w, 401, map[string]interface{}{"error": "missing Authorization header"})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid Authorization header format, expected 'Basic <credentials>'"})
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid base64 credentials"})
		return
	}

	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid credentials format"})
		return
	}

	username, password := credentials[0], credentials[1]
	if username != testBasicUser || password != testBasicPass {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid username or password"})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"authenticated": true,
		"auth_type":     "basic",
		"username":      username,
		"message":       "Basic authentication successful",
	})
}

func handleAPIKeyAuth(w http.ResponseWriter, r *http.Request) {
	// Check header first (X-API-Key)
	apiKey := r.Header.Get("X-API-Key")
	source := "header"

	// If not in header, check query param
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
		source = "query"
	}

	if apiKey == "" {
		writeJSON(w, 401, map[string]interface{}{"error": "missing API key (provide in X-API-Key header or api_key query param)"})
		return
	}

	if apiKey != testAPIKey {
		writeJSON(w, 401, map[string]interface{}{"error": "invalid API key"})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"authenticated": true,
		"auth_type":     "api_key",
		"key_source":    source,
		"message":       "API key authentication successful",
	})
}

// handleEcho echoes back the request body, headers, and query params.
// Useful for testing advanced body structures.
func handleEcho(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var body interface{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			writeJSON(w, 400, map[string]interface{}{"error": "invalid JSON body: " + err.Error()})
			return
		}
	}

	// Collect headers (exclude some internal ones)
	headers := make(map[string]interface{})
	for k, v := range r.Header {
		// Skip noisy headers
		if k == "Accept-Encoding" || k == "User-Agent" {
			continue
		}
		if len(v) == 1 {
			headers[k] = v[0]
		} else {
			headers[k] = v
		}
	}

	// Collect query params
	query := make(map[string]interface{})
	for k, v := range r.URL.Query() {
		if len(v) == 1 {
			query[k] = v[0]
		} else {
			query[k] = v
		}
	}

	writeJSON(w, 200, map[string]interface{}{
		"method":  r.Method,
		"path":    r.URL.Path,
		"body":    body,
		"headers": headers,
		"query":   query,
	})
}

// --- Helpers ---

func publishEvent(topic string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal kafka event: %v", err)
		return
	}

	msg := kafka.Message{
		Topic: topic,
		Value: data,
	}

	// Retry up to 3 times — first write may fail if topic auto-creation is in progress
	for attempt := 1; attempt <= 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := kafkaWriter.WriteMessages(ctx, msg)
		cancel()
		if err == nil {
			log.Printf("Published event to %s", topic)
			return
		}
		log.Printf("Attempt %d: failed to publish to %s: %v", attempt, topic, err)
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	log.Printf("Gave up publishing to %s after 3 attempts", topic)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
