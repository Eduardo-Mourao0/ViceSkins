package domain

// Wear liga a sigla usada na URL (FN) ao texto que a Steam grava em exterior.
var Wear = map[string]string{
	"FN": "Factory New",
	"MW": "Minimal Wear",
	"FT": "Field-Tested",
	"WW": "Well-Worn",
	"BS": "Battle-Scarred",
}

type ListingSort string

const (
	SortNewest    ListingSort = "newest"
	SortPriceAsc  ListingSort = "price_asc"
	SortPriceDesc ListingSort = "price_desc"
	SortFloatAsc  ListingSort = "float_asc"
)

func (s ListingSort) Valid() bool {
	switch s {
	case SortNewest, SortPriceAsc, SortPriceDesc, SortFloatAsc:
		return true
	}
	return false
}

const (
	DefaultPageSize = 30
	MaxPageSize     = 60
)

// ListingSearch são os filtros do mercado. Ponteiro nil = filtro desligado.
type ListingSearch struct {
	Text     string
	MinPrice *int64
	MaxPrice *int64
	Wear     []string // siglas: FN, MW...
	MinFloat *float64
	MaxFloat *float64
	Sort     ListingSort
	Page     int // começa em 1
	PageSize int
}

type Seller struct {
	ID          int64
	DisplayName string
	AvatarURL   string
}

// MarketListing é uma listing vista no mercado: com item e vendedor.
type MarketListing struct {
	Listing
	Seller Seller
}

type MarketPage struct {
	Listings []MarketListing
	Page     int
	PageSize int
	HasMore  bool
}
