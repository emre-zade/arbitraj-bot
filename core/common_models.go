package core

import "encoding/xml"

// --- CONFIG YAPILARI ---
type PazaramaConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type HepsiburadaConfig struct {
	MerchantID string `json:"merchant_id"`
	ApiSecret  string `json:"api_secret"`
	UserAgent  string `json:"user_agent"`
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

type PttProduct struct {
	UrunId      int64   `xml:"UrunId"`
	Barkod      string  `xml:"Barkod"`
	UrunAdi     string  `xml:"UrunAdi"`
	MevcutStok  int     `xml:"Miktar"` // API'den gelen stok
	MevcutFiyat float64 `xml:"KDVsiz"` // API'den gelen fiyat
	KdvOrani    int     `xml:"KDVOran"`
	Aktif       bool    `xml:"Aktif"`
	ResimURL    string  `xml:"UrunResim"`

	// Excel'den toplu yükleme (Upload) için ek alanlar
	StokKodu       string
	Fiyat          float64
	Stok           int
	HazirlikSuresi int
	Marka          string
	KategoriAdi    string //Excel'de ve debug'da okunabilirlik için eklendi
	KategoriId     int
	Aciklama       string
	Gorseller      []string
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

// MasterProduct: Excel'den yükleyeceğimiz temiz veriler için
type MasterProduct struct {
	SKU         string
	CleanTitle  string
	TargetBrand string
}

// --- GLOBAL KATEGORİ MODELLERİ (Merkezi Sistem İçin) ---

// PlatformCategory: DB'deki 'platform_categories' tablosunu temsil eder
type PlatformCategory struct {
	Platform     string // 'ptt', 'pazarama', 'hb'
	CategoryID   string // Platformun verdiği ID
	CategoryName string // Platformun verdiği isim
	ParentID     string // Üst kategori ID'si
	IsLeaf       bool   // En alt kategori mi? (Ürün yüklenebilir mi?)
}

// CategoryMapping: Senin Master kategorilerini platform ID'lerine bağlar
type CategoryMapping struct {
	MasterCategoryName string // Örn: 'Bebek Şampuanı'
	PttID              int
	PazaramaID         string
	HbID               string
}

// --- PAZARAMA KATEGORİ API MODELLERİ ---

type PazaramaCategoryResponse struct {
	Data    []PazaramaCategory `json:"data"`
	Success bool               `json:"success"`
	Message string             `json:"message"`
}

type PazaramaCategory struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	ParentID string             `json:"parentId"`
	IsLeaf   bool               `json:"leaf"`     // Dokümanda 'leaf' olarak geçer
	Children []PazaramaCategory `json:"children"` // Alt kategoriler (Recursive yapı)
}

//----------------------

type PazaramaCreateProductRequest struct {
	Products []PazaramaProductItem `json:"products"` // "items" değil "products" olmalı
}

type PazaramaProductItem struct {
	Code         string              `json:"code"`
	Name         string              `json:"name"`
	DisplayName  string              `json:"displayName"`
	Description  string              `json:"description"`
	BrandId      string              `json:"brandId"`   // BrandName değil ID istiyor
	GroupCode    string              `json:"groupCode"` // Zorunlu
	Desi         int                 `json:"desi"`      // Zorunlu
	StockCount   int                 `json:"stockCount"`
	StockCode    string              `json:"stockCode"`
	CurrencyType string              `json:"currencyType"` // "TRY"
	ListPrice    float64             `json:"listPrice"`
	SalePrice    float64             `json:"salePrice"`
	VatRate      int                 `json:"vatRate"`
	CategoryId   string              `json:"categoryId"`
	Images       []PazaramaImage     `json:"images"`
	Attributes   []PazaramaAttribute `json:"attributes"`
}

type PazaramaImage struct {
	Imageurl string `json:"imageurl"` // "url" değil "imageurl" (küçük harf!)
}

type PazaramaAttribute struct {
	AttributeId      string `json:"attributeId"`
	AttributeValueId string `json:"attributeValueId"`
}

type PazaramaBrandResponse struct {
	Data    []PazaramaBrand `json:"data"`
	Success bool            `json:"success"`
}

type PazaramaBrand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// HBProduct: Hepsiburada API'sinden gelen canlı veriler için
type HBProduct struct {
	HepsiburadaSku string  `json:"hepsiburadaSku"`
	MerchantSku    string  `json:"merchantSku"`
	Price          float64 `json:"price"`
	AvailableStock int     `json:"availableStock"`
	IsSalable      bool    `json:"isSalable"`
	ImageURL       string  `json:"imgURL"`
}

type HBListingResponse struct {
	Listings   []HBProduct `json:"listings"`
	TotalCount int         `json:"totalCount"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
}
