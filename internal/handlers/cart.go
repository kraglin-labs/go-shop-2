package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go-shop-2/internal/db"
	"go-shop-2/internal/models"
	appmw "go-shop-2/internal/middleware"
)

type CartHandler struct {
	DB *db.DB
}

func (h *CartHandler) GetCart(w http.ResponseWriter, r *http.Request) {
	userID := int32(appmw.GetClaims(r)["sub"].(float64))

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT c.id, p.id, p.name, p.price, p.image_url, c.quantity
		 FROM carts c
		 JOIN products p ON c.product_id = p.id
		 WHERE c.user_id = $1
		 ORDER BY c.created_at DESC`, userID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.CartItem
	for rows.Next() {
		var i models.CartItem
		rows.Scan(&i.CartID, &i.ProductID, &i.Name, &i.Price, &i.ImageURL, &i.Quantity)
		items = append(items, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *CartHandler) AddToCart(w http.ResponseWriter, r *http.Request) {
	var req models.AddToCartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}
	userID := int32(appmw.GetClaims(r)["sub"].(float64))

	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, `{"error":"transaction error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Lock product row
	var stock int32
	err = tx.QueryRow(r.Context(),
		`SELECT stock FROM products WHERE id = $1 FOR UPDATE`, req.ProductID).Scan(&stock)
	if err != nil {
		http.Error(w, `{"error":"product not found"}`, http.StatusNotFound)
		return
	}
	if stock <= 0 {
		http.Error(w, `{"error":"product out of stock"}`, http.StatusBadRequest)
		return
	}

	var cartID, currentQty int32
	err = tx.QueryRow(r.Context(),
		`SELECT id, quantity FROM carts WHERE user_id = $1 AND product_id = $2`,
		userID, req.ProductID).Scan(&cartID, &currentQty)

	if err != nil && err.Error() != "no rows in result set" {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	if err != nil { // no rows — new item
		if req.Quantity > stock {
			http.Error(w, `{"error":"not enough stock"}`, http.StatusBadRequest)
			return
		}
		_, err = tx.Exec(r.Context(),
			`INSERT INTO carts (user_id, product_id, quantity) VALUES ($1, $2, $3)`,
			userID, req.ProductID, req.Quantity)
	} else { // existing — merge
		newQty := currentQty + req.Quantity
		if newQty > stock {
			http.Error(w, `{"error":"not enough stock"}`, http.StatusBadRequest)
			return
		}
		_, err = tx.Exec(r.Context(),
			`UPDATE carts SET quantity = $1 WHERE id = $2`, newQty, cartID)
	}

	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, `{"error":"commit failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *CartHandler) UpdateCart(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateCartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}
	userID := int32(appmw.GetClaims(r)["sub"].(float64))
	cartID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if req.Quantity == 0 {
		tag, err := h.DB.Pool.Exec(r.Context(),
			`DELETE FROM carts WHERE id = $1 AND user_id = $2`, cartID, userID)
		if err != nil || tag.RowsAffected() == 0 {
			http.Error(w, `{"error":"cart item not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	var productStock int32
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT p.stock FROM products p
		 JOIN carts c ON p.id = c.product_id
		 WHERE c.id = $1 AND c.user_id = $2`, cartID, userID).Scan(&productStock)
	if err != nil {
		http.Error(w, `{"error":"cart item not found"}`, http.StatusNotFound)
		return
	}
	if req.Quantity > productStock {
		http.Error(w, `{"error":"not enough stock"}`, http.StatusBadRequest)
		return
	}

	tag, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE carts SET quantity = $1 WHERE id = $2 AND user_id = $3`,
		req.Quantity, cartID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		http.Error(w, `{"error":"cart item not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *CartHandler) RemoveFromCart(w http.ResponseWriter, r *http.Request) {
	userID := int32(appmw.GetClaims(r)["sub"].(float64))
	cartID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM carts WHERE id = $1 AND user_id = $2`, cartID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		http.Error(w, `{"error":"cart item not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
