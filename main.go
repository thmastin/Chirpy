package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/thmastin/Chirpy/internal/auth"
	"github.com/thmastin/Chirpy/internal/database"
)

var apiCfg apiConfig

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("error opening database: %v\n", err)
		os.Exit(1)
	}
	dbQueries := database.New(db)
	platform := os.Getenv("PLATFORM")
	tokenSecret := os.Getenv("SECRET")
	polkaKey := os.Getenv("POLKA_KEY")

	apiCfg = apiConfig{
		fileserverHits: atomic.Int32{},
		dbQueries:      dbQueries,
		platform:       platform,
		tokenSecret:    tokenSecret,
		polkaKey:       polkaKey,
	}

	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /admin/healthz", handlerHealthz)
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	mux.HandleFunc("POST /api/users", handlerAddUser)
	mux.HandleFunc("POST /api/chirps", handlerChirps)
	mux.HandleFunc("GET /api/chirps", handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", handlerGetChirp)
	mux.HandleFunc("POST /api/login", handlerLogin)
	mux.HandleFunc("POST /api/refresh", handlerRefresh)
	mux.HandleFunc("POST /api/revoke", handlerRevoke)
	mux.HandleFunc("PUT /api/users", handlerUpdateUserLogin)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", handlerDeleteChirp)
	mux.HandleFunc("POST /api/polka/webhooks", handlerSetRed)

	var s http.Server
	s.Handler = mux
	s.Addr = ":8080"

	s.ListenAndServe()

}

func handlerHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	tokenSecret    string
	polkaKey       string
}

func (apiCfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

func (apiCfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	content := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", apiCfg.fileserverHits.Load())
	w.Write([]byte(content))
}

func (apiCfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if apiCfg.platform != "dev" {
		respondWithError(w, 403, "forbidden")
		return
	}
	err := apiCfg.dbQueries.Reset(r.Context())
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("unable to reset users table: %v", err))
		return
	}
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	apiCfg.fileserverHits.Store(0)
}

func handlerChirps(w http.ResponseWriter, r *http.Request) {
	type paramaters struct {
		Body string `json:"body"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userID, err := auth.ValidateJWT(token, apiCfg.tokenSecret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := paramaters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	chirpLength := len(params.Body)

	if chirpLength > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	cleanedChirp := cleanChirpBody(params.Body)
	if cleanedChirp != params.Body {
		respondWithError(w, 422, "body contains bad words")
		return
	}

	args := database.CreateChirpParams{
		Body:   cleanedChirp,
		UserID: userID,
	}

	newChirp, err := apiCfg.dbQueries.CreateChirp(r.Context(), args)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("unable to create chirp: %v", err))
		return
	}
	chirp := Chirp{
		ID:        newChirp.ID,
		CreatedAt: newChirp.CreatedAt,
		UpdatedAt: newChirp.UpdatedAt,
		Body:      newChirp.Body,
		UserID:    newChirp.UserID,
	}
	respondWithJSON(w, 201, chirp)
}

func handlerAddUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
	}

	args := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}

	newUser, err := apiCfg.dbQueries.CreateUser(r.Context(), args)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("unable to create user: %v", err))
		return
	}
	user := User{
		ID:          newUser.ID,
		CreatedAt:   newUser.CreatedAt,
		UpdatedAt:   newUser.UpdatedAt,
		Email:       newUser.Email,
		IsChirpyRed: newUser.IsChirpyRed,
	}
	respondWithJSON(w, 201, user)
}

func handlerLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	apiUser, err := apiCfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	err = auth.CheckPasswordHash(params.Password, apiUser.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	token, err := auth.MakeJWT(apiUser.ID, apiCfg.tokenSecret, time.Hour)
	if err != nil {
		log.Printf("Error creating token: %v", err)
	}

	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("failed to create refresh token: %v", err)
		respondWithError(w, 500, "failed to create refresh token")
		return
	}

	currentTime := time.Now()
	expiryTime := currentTime.AddDate(0, 0, 60)

	refreshTokenParams := database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    apiUser.ID,
		ExpiresAt: expiryTime,
	}

	newRefreshToken, err := apiCfg.dbQueries.CreateRefreshToken(r.Context(), refreshTokenParams)
	if err != nil {
		log.Printf("failed to create refresh token: %v", err)
		respondWithError(w, 500, "failed to create refresh token")
	}

	user := User{
		ID:           apiUser.ID,
		CreatedAt:    apiUser.CreatedAt,
		UpdatedAt:    apiUser.UpdatedAt,
		Email:        apiUser.Email,
		Token:        token,
		RefreshToken: newRefreshToken.Token,
		IsChirpyRed:  apiUser.IsChirpyRed,
	}

	respondWithJSON(w, 200, user)

}
func handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	apiChirps := []Chirp{}
	dbChirps := []database.Chirp{}
	var err error
	s := r.URL.Query().Get("author_id")
	if s != "" {
		userID, err := uuid.Parse(s)
		if err != nil {
			log.Printf("Unable to pars author_id: %v", err)
			respondWithError(w, 500, "Server Error")
			return
		}
		dbChirps, err = apiCfg.dbQueries.GetChirpsByUser(r.Context(), userID)
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Unable to get chirps: %v", err))
		}
	} else {
		dbChirps, err = apiCfg.dbQueries.GetAllChirps(r.Context())
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("unable to retrieve chirps: %v", err))
			return
		}

	}

	sortParam := r.URL.Query().Get("sort")
	switch sortParam {
	case "asc":
		sort.Slice(dbChirps, func(i, j int) bool {
			return dbChirps[i].CreatedAt.Before(dbChirps[j].CreatedAt)
		})
	case "desc":
		sort.Slice(dbChirps, func(i, j int) bool {
			return dbChirps[i].CreatedAt.After(dbChirps[j].CreatedAt)
		})
	case "default":
		sort.Slice(dbChirps, func(i, j int) bool {
			return dbChirps[i].CreatedAt.Before(dbChirps[j].CreatedAt)
		})
	}
	if sortParam == "desc" {
		sort.Slice(dbChirps, func(i, j int) bool {
			// Assuming CreatedAt is a field in your database.Chirp struct that can be compared directly (e.g., time.Time or a sortable string format)
			return dbChirps[i].CreatedAt.After(dbChirps[j].CreatedAt)
		})
	}
	for i := range dbChirps {
		apiChirps = append(apiChirps, convertChirp(dbChirps[i]))
	}

	respondWithJSON(w, 200, apiChirps)
}

func handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("unable to parse request: %v", err))
		return
	}
	chirp, err := apiCfg.dbQueries.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("chirp not found: %v", err))
		return
	}
	apiChirp := convertChirp(chirp)
	respondWithJSON(w, 200, apiChirp)
}

func handlerRefresh(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	user, err := apiCfg.dbQueries.GetUserFromRefreshToken(r.Context(), token)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	newToken, err := auth.MakeJWT(user.ID, apiCfg.tokenSecret, time.Hour)
	if err != nil {
		log.Printf("Failed to generate JWT: %v", err)
		respondWithError(w, 500, "Failed to generate new token")
		return
	}
	respondWithJSON(w, 200, map[string]string{
		"token": newToken,
	})

}

func handlerRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	err = apiCfg.dbQueries.RevokeRefreshToken(r.Context(), token)
	if err != nil {
		log.Printf("Unable to revoke refresh token: %v", err)
	}
	respondWithJSON(w, 204, nil)
}

func handlerUpdateUserLogin(w http.ResponseWriter, r *http.Request) {
	type paramaters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userID, err := auth.ValidateJWT(token, apiCfg.tokenSecret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := paramaters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	newHashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		respondWithError(w, 500, "Server Error")
		return
	}

	args := database.UpdateUserLoginParams{
		Email:          params.Email,
		HashedPassword: newHashedPassword,
		ID:             userID,
	}

	updatedUser, err := apiCfg.dbQueries.UpdateUserLogin(r.Context(), args)
	if err != nil {
		log.Printf("Error updating user: %v", err)
		respondWithError(w, 500, "Server Error")
	}

	user := User{
		ID:          updatedUser.ID,
		CreatedAt:   updatedUser.CreatedAt,
		UpdatedAt:   updatedUser.UpdatedAt,
		Email:       updatedUser.Email,
		IsChirpyRed: updatedUser.IsChirpyRed,
	}
	respondWithJSON(w, 200, user)
}

func handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	userID, err := auth.ValidateJWT(token, apiCfg.tokenSecret)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	chirpID := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("unable to parse request: %v", err))
		return
	}
	chirp, err := apiCfg.dbQueries.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("chirp not found: %v", err))
		return
	}

	if userID != chirp.UserID {
		respondWithError(w, 403, "Forbidden")
		return
	}

	err = apiCfg.dbQueries.DeleteChirp(r.Context(), chirp.ID)
	if err != nil {
		log.Printf("Unalbe to delete chirp: %v", err)
		respondWithError(w, 500, "Server Error")
		return
	}
	respondWithJSON(w, 204, nil)

}

func handlerSetRed(w http.ResponseWriter, r *http.Request) {
	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorized")
		return
	}
	if apiKey != apiCfg.polkaKey {
		respondWithError(w, 401, "Unauthorize")
	}

	type UserUpgradedEvent struct {
		Event string `json:"event"`
		Data  struct {
			UserID uuid.UUID `json:"user_id"`
		}
	}

	var event UserUpgradedEvent

	err = json.NewDecoder(r.Body).Decode(&event)
	if err != nil {
		log.Printf("Error decoding request: %v", err)
		respondWithError(w, 500, "Server Error")
		return
	}

	if event.Event != "user.upgraded" {
		respondWithJSON(w, 204, nil)
		return
	}

	err = apiCfg.dbQueries.SetUserToRed(r.Context(), event.Data.UserID)
	if err != nil {
		respondWithError(w, 404, "User not found")
		return
	}
	respondWithJSON(w, 204, nil)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorMsg struct {
		Error string `json:"error"`
	}

	respBody := errorMsg{
		Error: msg,
	}

	dat, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling data: %s", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(500)
		w.Write([]byte("Internal server error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, 500, "Internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func cleanChirpBody(s string) string {
	words := strings.Split(s, " ")
	for i := range words {
		word := strings.ToLower(words[i])
		switch word {
		case "kerfuffle", "sharbert", "fornax":
			words[i] = "****"
		}
	}
	return strings.Join(words, " ")
}

func convertChirp(c database.Chirp) Chirp {
	return Chirp{
		ID:        c.ID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Body:      c.Body,
		UserID:    c.UserID,
	}
}

type RefreshToken struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uuid.UUID `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	RevokeAt  time.Time `json:"RevokeAt"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}
