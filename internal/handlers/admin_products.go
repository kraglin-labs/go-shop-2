package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"go-shop-2/internal/db"
	"go-shop-2/internal/models"
)

type AdminProductHandler struct {
	DB          *db.DB
	SupabaseURL string
	SupabaseKey string
}

func (h *AdminProductHandler) List(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	var total int64
	_ = h.DB.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM products`).Scan(&total)

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, name, price, image_url, stock, description, size, category
		 FROM products ORDER BY id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
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

func (h *AdminProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}

	var p models.Product
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO products (name, price, image_url, stock, description, size, category)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, name, price, image_url, stock, description, size, category`,
		req.Name, req.Price, req.ImageURL, req.Stock, req.Description, req.Size, req.Category,
	).Scan(&p.ID, &p.Name, &p.Price, &p.ImageURL, &p.Stock, &p.Description, &p.Size, &p.Category)

	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func (h *AdminProductHandler) CreateWithUpload(w http.ResponseWriter, r *http.Request) {
	if h.SupabaseURL == "" || h.SupabaseKey == "" {
		http.Error(w, `{"error":"storage not configured"}`, http.StatusInternalServerError)
		return
	}

	// 5MB max
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		http.Error(w, `{"error":"request too large (max 5MB)"}`, http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	name := r.FormValue("name")
	priceStr := r.FormValue("price")
	stockStr := r.FormValue("stock")
	description := r.FormValue("description")
	size := r.FormValue("size")
	category := r.FormValue("category")

	if name == "" || priceStr == "" || stockStr == "" || description == "" || size == "" || category == "" {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}
	price, _ := strconv.ParseInt(priceStr, 10, 64)
	stock, _ := strconv.ParseInt(stockStr, 10, 64)

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, `{"error":"image required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Verify MIME type
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	file.Seek(0, io.SeekStart)
	mime := http.DetectContentType(buf[:n])
	if !strings.HasPrefix(mime, "image/") {
		http.Error(w, `{"error":"file must be an image"}`, http.StatusBadRequest)
		return
	}

	// Upload to Supabase
	uniqueName := fmt.Sprintf("%s_%s", uuid.NewString(), header.Filename)
	endpoint := fmt.Sprintf("%s/storage/v1/object/product-images/%s", h.SupabaseURL, uniqueName)

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	hh := make(textproto.MIMEHeader)
	hh.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", header.Filename))
	hh.Set("Content-Type", mime)
	part, _ := mw.CreatePart(hh)
	io.Copy(part, file)
	mw.Close()

	req2, _ := http.NewRequest("POST", endpoint, &b)
	req2.Header.Set("apikey", h.SupabaseKey)
	req2.Header.Set("Authorization", "Bearer "+h.SupabaseKey)
	req2.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req2)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, `{"error":"image upload failed"}`, http.StatusInternalServerError)
		return
	}
	resp.Body.Close()

	imageURL := fmt.Sprintf("%s/storage/v1/object/public/product-images/%s", h.SupabaseURL, uniqueName)

	var p models.Product
	err = h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO products (name, price, image_url, stock, description, size, category)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, name, price, image_url, stock, description, size, category`,
		name, price, imageURL, stock, description, size, category,
	).Scan(&p.ID, &p.Name, &p.Price, &p.ImageURL, &p.Stock, &p.Description, &p.Size, &p.Category)

	if err != nil {
		http.Error(w, `{"error":"database insertion failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func (h *AdminProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	var req models.UpdateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := models.Validate.Struct(req); err != nil {
		http.Error(w, `{"error":"`+validationError(err)+`"}`, http.StatusBadRequest)
		return
	}

	tag, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE products SET
			name = COALESCE($1, name),
			price = COALESCE($2, price),
			image_url = COALESCE($3, image_url),
			stock = COALESCE($4, stock),
			description = COALESCE($5, description),
			size = COALESCE($6, size),
			category = COALESCE($7, category)
		 WHERE id = $8`,
		req.Name, req.Price, req.ImageURL, req.Stock, req.Description, req.Size, req.Category, id,
	)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, `{"error":"product not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AdminProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	var hasOrders bool
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM order_items WHERE product_id = $1)`, id).Scan(&hasOrders)
	if hasOrders {
		http.Error(w, `{"error":"cannot delete product with existing orders"}`, http.StatusConflict)
		return
	}

	tag, err := h.DB.Pool.Exec(r.Context(), `DELETE FROM products WHERE id = $1`, id)
	if err != nil || tag.RowsAffected() == 0 {
		http.Error(w, `{"error":"product not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
