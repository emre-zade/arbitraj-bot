package core

import "encoding/xml"

// --- CONFIG YAPILARI ---
type PazaramaConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type HepsiburadaConfig struct {
	MerchantID string `json:"merchant_id"`
	ApiKey     string `json:"api_key"`
	ApiSecret  string `json:"api_secret"`
}

type PttConfig struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	PanelEmail  string `json:"panel_email"`
	PanelPasswd string `json:"panel_psswd"`
	Token       string `json:"token"`
}

type Config struct {
	Pazarama    PazaramaConfig    `json:"pazarama"`
	Hepsiburada HepsiburadaConfig `json:"hepsiburada"`
	Ptt         PttConfig         `json:"ptt"`
}

// --- PAZARAMA MODELLERİ ---
type PazaramaAuthResponse struct {
	Data struct {
		AccessToken string `json:"accessToken"`
	} `json:"data"`
}

type PazaramaProduct struct {
	Name       string  `json:"name"`
	Code       string  `json:"code"`
	StockCount int     `json:"stockCount"`
	SalePrice  float64 `json:"salePrice"`
	BrandName  string  `json:"brandName"`
}

type PazaramaProductResponse struct {
	Data    []PazaramaProduct `json:"data"`
	Success bool              `json:"success"`
}

// PttProduct: Hem XML'den veri çekerken hem REST API ile güncellerken kullanacağız
type PttProduct struct {
	UrunId      int64   `xml:"UrunId"` // REST API'nin beklediği product_id burası
	Barkod      string  `xml:"Barkod"`
	UrunAdi     string  `xml:"UrunAdi"`
	MevcutStok  int     `xml:"Miktar"`
	MevcutFiyat float64 `xml:"KDVsiz"`
	KdvOrani    int     `xml:"KDVOran"`
	Aktif       bool    `xml:"Aktif"`
	ResimURL    string  `xml:"UrunResim"`
}

type PttStockPriceUpdate struct {
	ProductID string  // REST API için şart olan ID (H sütunu)
	Barcode   string  // Loglarda görmek için
	Stock     int     // Yeni miktar (quantity)
	Price     float64 // Yeni KDV hariç fiyat (vat_excluded_price)
}

type PttListResponse struct {
	XMLName  xml.Name     `xml:"Envelope"`
	Products []PttProduct `xml:"Body>StokKontrolListesiResponse>StokKontrolListesiResult>StokKontrolDetay"`
}

type PttLoginRequest struct {
	Email    string `json:"panel_email"`
	Password string `json:"panel_passwd"`
}

type PttVerifyOTPRequest struct {
	OtpCode     string `json:"otpCode"`
	OtpId       string `json:"otpId"`
	PreOtpToken string `json:"preOtpToken"`
}

type PttLoginResponse struct {
	Data struct {
		OtpRequired  bool   `json:"otpRequired"`
		PreOtpToken  string `json:"preOtpToken"`
		OtpId        string `json:"otpId"`
		AccessToken  string `json:"accessToken"`  // Bazen gövdede gelebilir
		RefreshToken string `json:"refreshToken"` // Bazen gövdede gelebilir
	} `json:"data"`
	IsSuccess bool `json:"isSuccess"`
}

// HBProduct: Hepsiburada API'sinden gelen canlı veriler için
type HBProduct struct {
	SKU      string
	Barcode  string
	Price    float64
	Stock    int
	ImageURL string
}

// MasterProduct: Excel'den yükleyeceğimiz temiz veriler için
type MasterProduct struct {
	SKU         string
	CleanTitle  string
	TargetBrand string
}
