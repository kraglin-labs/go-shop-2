package models

import (
	"time"

	"github.com/go-playground/validator/v10"
)

var Validate = validator.New()

type User struct {
	ID       int32   `json:"id"`
	Email    string  `json:"email"`
	Role     string  `json:"role"`
	FullName *string `json:"full_name,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	Address  *string `json:"address,omitempty"`
}

type Claims struct {
	Sub   int32  `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type RegisterRequest struct {
	Email    string  `json:"email" validate:"required,email"`
	Password string  `json:"password" validate:"required,min=6,max=72"`
	FullName *string `json:"full_name" validate:"required,min=1,max=100"`
	Phone    *string `json:"phone,omitempty" validate:"omitempty,e164"`
	Address  *string `json:"address,omitempty" validate:"omitempty,min=5,max=500"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type Product struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	Price       int64  `json:"price"`
	ImageURL    string `json:"image_url"`
	Stock       int32  `json:"stock"`
	Description string `json:"description"`
	Size        string `json:"size"`
	Category    string `json:"category"`
}

type PaginationMeta struct {
	Page       int32 `json:"page"`
	PageSize   int32 `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int32 `json:"total_pages"`
}

type PaginatedProducts struct {
	Data []Product      `json:"data"`
	Meta PaginationMeta `json:"meta"`
}


type CreateProductRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=255"`
	Price       int64   `json:"price" validate:"required,min=0"`
	Stock       int32   `json:"stock" validate:"required,min=0"`
	ImageURL    string  `json:"image_url" validate:"required,url"`
	Description *string `json:"description,omitempty"`
	Size        *string `json:"size,omitempty" validate:"omitempty,max=100"`
	Category    string  `json:"category" validate:"required,min=1,max=100"`
}

type UpdateProductRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Price       *int64  `json:"price,omitempty" validate:"omitempty,min=0"`
	Stock       *int32  `json:"stock,omitempty" validate:"omitempty,min=0"`
	ImageURL    *string `json:"image_url,omitempty" validate:"omitempty,url"`
	Description *string `json:"description,omitempty"`
	Size        *string `json:"size,omitempty" validate:"omitempty,max=100"`
	Category    *string `json:"category,omitempty" validate:"omitempty,min=1,max=100"`
}

type CartItem struct {
	CartID    int32  `json:"cart_id"`
	ProductID int32  `json:"product_id"`
	Name      string `json:"name"`
	Price     int64  `json:"price"`
	ImageURL  string `json:"image_url"`
	Quantity  int32  `json:"quantity"`
}

type AddToCartRequest struct {
	ProductID int32 `json:"product_id" validate:"required,min=1"`
	Quantity  int32 `json:"quantity" validate:"required,min=1"`
}

type UpdateCartRequest struct {
	Quantity int32 `json:"quantity" validate:"required,min=0"`
}

type Order struct {
	ID              int32     `json:"id"`
	TotalAmount     int64     `json:"total_amount"`
	Status          string    `json:"status"`
	ShippingAddress string    `json:"shipping_address"`
	ContactPhone    string    `json:"contact_phone"`
	CreatedAt       time.Time `json:"created_at"`
}

type OrderItem struct {
	ID          int32  `json:"id"`
	ProductID   int32  `json:"product_id"`
	Name        string `json:"name"`
	ImageURL    string `json:"image_url"`
	Quantity    int32  `json:"quantity"`
	PriceAtTime int64  `json:"price_at_time"`
}

type OrderDetail struct {
	Order Order       `json:"order"`
	Items []OrderItem `json:"items"`
}

type CreateOrderRequest struct {
	ShippingAddress string `json:"shipping_address" validate:"required,min=5,max=500"`
	ContactPhone    string `json:"contact_phone" validate:"required,e164"`
}

type AdminOrder struct {
	ID              int32     `json:"id"`
	UserEmail       string    `json:"user_email"`
	TotalAmount     int64     `json:"total_amount"`
	Status          string    `json:"status"`
	ShippingAddress *string   `json:"shipping_address,omitempty"`
	ContactPhone    *string   `json:"contact_phone,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	ItemCount       int64     `json:"item_count"`
}

type PaginatedOrders struct {
	Data []Order        `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

type PaginatedAdminOrders struct {
	Data []AdminOrder   `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

type UpdateOrderStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=pending processing shipped delivered cancelled"`
}

type PaginatedOrderDetails struct {
	Data []OrderDetail  `json:"data"`
	Meta PaginationMeta `json:"meta"`
}


type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required,len=64"`
	NewPassword string `json:"new_password" validate:"required,min=6,max=72"`
}
