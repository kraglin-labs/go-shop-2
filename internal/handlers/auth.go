package handlers

import (
	"crypto/rand"
	"crypto/sha256"
//	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"go-shop-2/internal/db"
	"go-shop-2/internal/models"
)

type AuthHandler struct {
	DB     *db.DB
	Secret string
}

func (h *AuthHandler) hash(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func (h *AuthHandler) check(pw, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func (h *AuthHandler) token(userID int32, email, role string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"role":  role,
		"exp":   time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.Secret))
}

func validationError(err error) string {
	if errs, ok := err.(validator.ValidationErrors); ok {
		var msgs []string
		for _, e := range errs {
			switch e.Tag() {
			case "required":
				msgs = append(msgs, e.Field()+" is required")
			case "email":
				msgs = append(msgs, e.Field()+" must be a valid email")
			case "min":
				msgs = append(msgs, e.Field()+" is too short")
			case "max":
				msgs = append(msgs, e.Field()+" is too long")
			case "len":
				msgs = append(msgs, e.Field()+" has invalid length")
			case "e164":
				msgs = append(msgs, e.Field()+" must be a valid phone number")
			case "url":
				msgs = append(msgs, e.Field()+" must be a valid URL")
			default:
				msgs = append(msgs, e.Field()+" is invalid")
			}
		}
		return strings.Join(msgs, "; ")
	}
	return err.Error()
}

func generateResetToken() (rawToken, tokenHash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	rawToken = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(rawToken))
	tokenHash = hex.EncodeToString(sum[:])
	return rawToken, tokenHash, nil
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}

	hash, err := h.hash(req.Password)
	if err != nil {
		http.Error(w, `{"error":"hash failed"}`, http.StatusInternalServerError)
		return
	}

	var user models.User
	err = h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO users (email, password_hash, full_name, phone, address, role)
		 VALUES ($1, $2, $3, $4, $5, 'user')
		 RETURNING id, email, role, full_name, phone, address`,
		req.Email, hash, req.FullName, req.Phone, req.Address,
	).Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Phone, &user.Address)

	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, `{"error":"email already exists"}`, http.StatusConflict)
			return
		}
		log.Printf("REGISTER DB ERROR: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	tok, _ := h.token(user.ID, user.Email, user.Role)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.AuthResponse{Token: tok, User: user})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}

	var user models.User
	var hash string
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, email, role, full_name, phone, address, password_hash
		 FROM users WHERE email = $1`, req.Email,
	).Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Phone, &user.Address, &hash)

	if err == pgx.ErrNoRows || !h.check(req.Password, hash) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("LOGIN DB ERROR: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	tok, _ := h.token(user.ID, user.Email, user.Role)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.AuthResponse{Token: tok, User: user})
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req models.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}

	// Always return the same message to prevent user enumeration
	var userID int32
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1`, req.Email).Scan(&userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "if the email exists, a reset token has been generated",
		})
		return
	}

	// Delete old unused tokens for this user
	_, _ = h.DB.Pool.Exec(r.Context(),
		`DELETE FROM password_resets WHERE user_id = $1 AND used = FALSE`, userID)

	token, tokenHash, err := generateResetToken()
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	_, err = h.DB.Pool.Exec(r.Context(),
		`INSERT INTO password_resets (user_id, token_hash, expires_at)
		 VALUES ($1, $2, NOW() + INTERVAL '1 hour')`,
		userID, tokenHash)
	if err != nil {
		log.Printf("FORGOT PASSWORD DB ERROR: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// IN PRODUCTION: send token via email. Here we return it for testing.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "reset token generated (in production, this would be emailed)",
		"token":   token,
	})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req models.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}

	// Hash provided token and compare
	sum := sha256.Sum256([]byte(req.Token))
	hashHex := hex.EncodeToString(sum[:])

	var userID int32
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM password_resets
		 WHERE token_hash = $1 AND used = FALSE AND expires_at > NOW()`,
		hashHex).Scan(&userID)
	if err != nil {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusBadRequest)
		return
	}

	pwHash, err := h.hash(req.NewPassword)
	if err != nil {
		http.Error(w, `{"error":"hash failed"}`, http.StatusInternalServerError)
		return
	}

	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, `{"error":"transaction error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`UPDATE users SET password_hash = $1 WHERE id = $2`, pwHash, userID)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(r.Context(),
		`UPDATE password_resets SET used = TRUE WHERE token_hash = $1`, hashHex)
	if err != nil {
		http.Error(w, `{"error":"token update failed"}`, http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, `{"error":"commit failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "password updated successfully"})
}
