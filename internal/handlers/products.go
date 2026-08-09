package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go-shop-2/internal/db"
	"go-shop-2/internal/models"
)

type ProductHandler struct {
	DB *db.DB
}

func (h *ProductHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page := int32(1)
	pageSize := int32(20)
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 0 {
		page = int32(p)
	}
	if ps, err := strconv.Atoi(q.Get("page_size")); err == nil && ps > 0 && ps <= 100 {
		pageSize = int32(ps)
	}
	offset := (page - 1) * pageSize

	search := q.Get("search")
	category := q.Get("category")
	minPriceStr := q.Get("min_price")
	maxPriceStr := q.Get("max_price")

	where := "WHERE stock > 0"
	args := []interface{}{}
	argCount := 0

	addCond := func(cond string, val interface{}) {
		argCount++
		where += " AND " + cond
		args = append(args, val)
	}

	if search != "" {
		argCount++
		where += " AND (name ILIKE $" + strconv.Itoa(argCount) + " OR description ILIKE $" + strconv.Itoa(argCount) + ")"
		args = append(args, "%"+search+"%")
	}
	if category != "" {
		addCond("category = $"+strconv.Itoa(argCount+1), category)
	}
	if minPriceStr != "" {
		if v, err := strconv.ParseInt(minPriceStr, 10, 64); err == nil && v >= 0 {
			addCond("price >= $"+strconv.Itoa(argCount+1), v)
		}
	}
	if maxPriceStr != "" {
		if v, err := strconv.ParseInt(maxPriceStr, 10, 64); err == nil && v >= 0 {
			addCond("price <= $"+strconv.Itoa(argCount+1), v)
		}
	}

	countSQL := "SELECT COUNT(*) FROM products " + where
	var total int64
	_ = h.DB.Pool.QueryRow(r.Context(), countSQL, args...).Scan(&total)

	fetchSQL := "SELECT id, name, price, image_url, stock, description, size, category FROM products " + where +
		" ORDER BY id DESC LIMIT $" + strconv.Itoa(argCount+1) + " OFFSET $" + strconv.Itoa(argCount+2)
	args = append(args, pageSize, offset)

	rows, err := h.DB.Pool.Query(r.Context(), fetchSQL, args...)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		rows.Scan(&p.ID, &p.Name, &p.Price, &p.ImageURL, &p.Stock, &p.Description, &p.Size, &p.Category)
		products = append(products, p)
	}

	totalPages := int32(total / int64(pageSize))
	if total%int64(pageSize) > 0 {
		totalPages++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.PaginatedProducts{
		Data: products,
		Meta: models.PaginationMeta{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *ProductHandler) GetProductByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	var p models.Product
	err = h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, name, price, image_url, stock, description, size, category
		 FROM products WHERE id = $1 AND stock > 0`, id,
	).Scan(&p.ID, &p.Name, &p.Price, &p.ImageURL, &p.Stock, &p.Description, &p.Size, &p.Category)

	if err != nil {
		http.Error(w, `{"error":"product not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (h *ProductHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT DISTINCT category FROM products WHERE stock > 0 ORDER BY category ASC`)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		cats = append(cats, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cats)
}
