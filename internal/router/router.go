package router

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"go-shop-2/internal/db"
	"go-shop-2/internal/handlers"
	appmw "go-shop-2/internal/middleware"
)

func New(database *db.DB, jwtSecret, adminAPIKey, supabaseURL, supabaseKey string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	notFoundBody := []byte(`{"error":"not found"}`)
	notFound := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write(notFoundBody)
	}
	r.NotFound(notFound)
	r.MethodNotAllowed(notFound)

	auth := &handlers.AuthHandler{DB: database, Secret: jwtSecret}

	// Rate limiter: 5 requests per minute per IP, returns JSON
	authLimiter := httprate.Limit(5, time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded, try again later"}`))
		}),
	)

	// Public auth (rate limited)
	r.With(authLimiter).Post("/api/auth/register", auth.Register)
	r.With(authLimiter).Post("/api/auth/login", auth.Login)
	r.With(authLimiter).Post("/api/auth/forgot-password", auth.ForgotPassword)
	r.With(authLimiter).Post("/api/auth/reset-password", auth.ResetPassword)

	// Public products
	products := &handlers.ProductHandler{DB: database}
	r.Get("/api/products", products.GetProducts)
	r.Get("/api/products/{id}", products.GetProductByID)
	r.Get("/api/categories", products.GetCategories)

	// Protected (JWT)
	r.Group(func(protected chi.Router) {
		protected.Use(appmw.JWT(jwtSecret))

		protected.Get("/api/auth/me", handlers.Me)

		cart := &handlers.CartHandler{DB: database}
		protected.Get("/api/cart", cart.GetCart)
		protected.Post("/api/cart", cart.AddToCart)
		protected.Put("/api/cart/{id}", cart.UpdateCart)
		protected.Delete("/api/cart/{id}", cart.RemoveFromCart)

		orders := &handlers.OrderHandler{DB: database}
		protected.Get("/api/orders", orders.GetOrders)
		protected.Post("/api/orders", orders.CreateOrder)
		protected.Get("/api/orders/{id}", orders.GetOrderDetail)
		protected.Delete("/api/orders/{id}", orders.CancelOrder)
	})

	// Admin
	adminProducts := &handlers.AdminProductHandler{
		DB:          database,
		SupabaseURL: supabaseURL,
		SupabaseKey: supabaseKey,
	}
	adminOrders := &handlers.AdminOrderHandler{DB: database}

	r.Group(func(admin chi.Router) {
		admin.Use(appmw.AdminAuth(adminAPIKey, notFoundBody))
		admin.Get("/sys/v2/stats", handlers.AdminStats)
		admin.Get("/sys/v2/products", adminProducts.List)
		admin.Post("/sys/v2/products", adminProducts.Create)
		admin.Post("/sys/v2/products/upload", adminProducts.CreateWithUpload)
		admin.Put("/sys/v2/products/{id}", adminProducts.Update)
		admin.Delete("/sys/v2/products/{id}", adminProducts.Delete)

		admin.Get("/sys/v2/orders", adminOrders.List)
		admin.Get("/sys/v2/orders/{id}", adminOrders.GetDetail)
		admin.Put("/sys/v2/orders/{id}/status", adminOrders.UpdateStatus)
		admin.Delete("/sys/v2/orders/{id}", adminOrders.CancelOrder)
	})

	return r
}
