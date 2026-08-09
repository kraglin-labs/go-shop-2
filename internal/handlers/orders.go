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

type OrderHandler struct {
	DB *db.DB
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req models.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}
	userID := int32(appmw.GetClaims(r)["sub"].(float64))

	// Fetch cart items
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT p.id, p.price, c.quantity
		 FROM carts c
		 JOIN products p ON c.product_id = p.id
		 WHERE c.user_id = $1`, userID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	type line struct {
		productID int32
		price     int64
		qty       int32
	}
	var lines []line
	var total int64
	for rows.Next() {
		var l line
		rows.Scan(&l.productID, &l.price, &l.qty)
		lines = append(lines, l)
		total += l.price * int64(l.qty)
	}
	rows.Close()

	if len(lines) == 0 {
		http.Error(w, `{"error":"cart is empty"}`, http.StatusBadRequest)
		return
	}

	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, `{"error":"transaction error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Decrement stock atomically
	for _, l := range lines {
		tag, err := tx.Exec(r.Context(),
			`UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1`,
			l.qty, l.productID)
		if err != nil || tag.RowsAffected() == 0 {
			http.Error(w, `{"error":"insufficient stock for one or more items"}`, http.StatusBadRequest)
			return
		}
	}

	// Create order
	var order models.Order
	err = tx.QueryRow(r.Context(),
		`INSERT INTO orders (user_id, total_amount, status, shipping_address, contact_phone)
		 VALUES ($1, $2, 'pending', $3, $4)
		 RETURNING id, total_amount, status, shipping_address, contact_phone, created_at`,
		userID, total, req.ShippingAddress, req.ContactPhone,
	).Scan(&order.ID, &order.TotalAmount, &order.Status, &order.ShippingAddress, &order.ContactPhone, &order.CreatedAt)
	if err != nil {
		http.Error(w, `{"error":"order creation failed"}`, http.StatusInternalServerError)
		return
	}

	// Insert order items
	for _, l := range lines {
		_, err = tx.Exec(r.Context(),
			`INSERT INTO order_items (order_id, product_id, quantity, price_at_time)
			 VALUES ($1, $2, $3, $4)`,
			order.ID, l.productID, l.qty, l.price)
		if err != nil {
			http.Error(w, `{"error":"failed to save order items"}`, http.StatusInternalServerError)
			return
		}
	}

	// Clear cart
	_, _ = tx.Exec(r.Context(), `DELETE FROM carts WHERE user_id = $1`, userID)

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, `{"error":"commit failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(order)
}

func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID := int32(appmw.GetClaims(r)["sub"].(float64))
	page, pageSize := parsePagination(r)

	var total int64
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM orders WHERE user_id = $1`, userID).Scan(&total)

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, total_amount, status, shipping_address, contact_phone, created_at
		 FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, pageSize, (page-1)*pageSize)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		rows.Scan(&o.ID, &o.TotalAmount, &o.Status, &o.ShippingAddress, &o.ContactPhone, &o.CreatedAt)
		orders = append(orders, o)
	}

	totalPages := int32(total / int64(pageSize))
	if total%int64(pageSize) > 0 {
		totalPages++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.PaginatedOrders{
		Data: orders,
		Meta: models.PaginationMeta{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *OrderHandler) GetOrderDetail(w http.ResponseWriter, r *http.Request) {
	userID := int32(appmw.GetClaims(r)["sub"].(float64))
	orderID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var order models.Order
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, total_amount, status, shipping_address, contact_phone, created_at
		 FROM orders WHERE id = $1 AND user_id = $2`, orderID, userID,
	).Scan(&order.ID, &order.TotalAmount, &order.Status, &order.ShippingAddress, &order.ContactPhone, &order.CreatedAt)
	if err != nil {
		http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
		return
	}

	rows, _ := h.DB.Pool.Query(r.Context(),
		`SELECT oi.id, oi.product_id, p.name, p.image_url, oi.quantity, oi.price_at_time
		 FROM order_items oi
		 JOIN products p ON oi.product_id = p.id
		 WHERE oi.order_id = $1`, orderID)
	defer rows.Close()

	var items []models.OrderItem
	for rows.Next() {
		var i models.OrderItem
		rows.Scan(&i.ID, &i.ProductID, &i.Name, &i.ImageURL, &i.Quantity, &i.PriceAtTime)
		items = append(items, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.OrderDetail{Order: order, Items: items})
}

func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	userID := int32(appmw.GetClaims(r)["sub"].(float64))
	orderID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, `{"error":"transaction error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var status string
	err = tx.QueryRow(r.Context(),
		`SELECT status FROM orders WHERE id = $1 AND user_id = $2 FOR UPDATE`,
		orderID, userID).Scan(&status)
	if err != nil {
		http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
		return
	}
	if status != "pending" && status != "processing" {
		http.Error(w, `{"error":"order cannot be cancelled"}`, http.StatusBadRequest)
		return
	}

	rows, _ := tx.Query(r.Context(),
		`SELECT product_id, quantity FROM order_items WHERE order_id = $1`, orderID)
	var restocks []struct {
		pid int32
		qty int32
	}
	for rows.Next() {
		var rs struct{ pid, qty int32 }
		rows.Scan(&rs.pid, &rs.qty)
		restocks = append(restocks, rs)
	}
	rows.Close()

	for _, rs := range restocks {
		_, _ = tx.Exec(r.Context(),
			`UPDATE products SET stock = stock + $1 WHERE id = $2`, rs.qty, rs.pid)
	}

	_, err = tx.Exec(r.Context(),
		`UPDATE orders SET status = 'cancelled' WHERE id = $1`, orderID)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, `{"error":"commit failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
