package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go-shop-2/internal/db"
	"go-shop-2/internal/models"
)

type AdminOrderHandler struct {
	DB *db.DB
}

func (h *AdminOrderHandler) List(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)

	var total int64
	_ = h.DB.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM orders`).Scan(&total)

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT o.id, u.email, o.total_amount, o.status,
		        o.shipping_address, o.contact_phone, o.created_at, COUNT(oi.id)
		 FROM orders o
		 JOIN users u ON o.user_id = u.id
		 LEFT JOIN order_items oi ON o.id = oi.order_id
		 GROUP BY o.id, u.email, o.total_amount, o.status, o.shipping_address, o.contact_phone, o.created_at
		 ORDER BY o.created_at DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orders []models.AdminOrder
	for rows.Next() {
		var o models.AdminOrder
		rows.Scan(&o.ID, &o.UserEmail, &o.TotalAmount, &o.Status, &o.ShippingAddress, &o.ContactPhone, &o.CreatedAt, &o.ItemCount)
		orders = append(orders, o)
	}

	totalPages := int32(total / int64(pageSize))
	if total%int64(pageSize) > 0 {
		totalPages++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.PaginatedAdminOrders{
		Data: orders,
		Meta: models.PaginationMeta{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *AdminOrderHandler) GetDetail(w http.ResponseWriter, r *http.Request) {
	orderID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var order models.Order
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, total_amount, status, shipping_address, contact_phone, created_at
		 FROM orders WHERE id = $1`, orderID,
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

func (h *AdminOrderHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateOrderStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}
	orderID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tag, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE orders SET status = $1 WHERE id = $2`, req.Status, orderID)
	if err != nil || tag.RowsAffected() == 0 {
		http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AdminOrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, `{"error":"transaction error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var status string
	err = tx.QueryRow(r.Context(),
		`SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&status)
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
