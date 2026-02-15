package kiosbank

// Response code constants
const (
	RCSuccess              = "00"
	RCUndefinedError       = "01"
	RCFormatError          = "02"
	RCBillerNoResponse     = "03"
	RCNoResponseFromBiller = "04"
	RCNoResponseFromHost   = "05"
	RCCannotProcess        = "10"
	RCInsufficientBalance  = "12"
	RCStorageIssue         = "14"
	RCUnknownProduct       = "15"
	RCTransactionFailed    = "17"
	RCNotRegistered        = "18"
	RCDataNotFound         = "19"
	RCCannotTransact       = "27"
	RCSessionExpired       = "34"
	RCAdminError           = "37"
	RCBillerLinkDown       = "38"
	RCTimeout              = "39"
	RCUnknownMessage       = "40"
	RCNotAuthorized        = "41"
	RCInvalidPrice         = "42"
	RCExpired              = "46"
	RCPayAtOffice          = "60"
	RCInquiryRequired      = "61"
	RCBillNotAvailable     = "62"
	RCAlreadyPaid          = "64"
	RCInvalidCustomer      = "65"
	RCProcessFailure       = "67"
	RCDailyLimitReached    = "68"
	RCNumberNotAllowed     = "69"
	RCInvalidAmount        = "70"
	RCProcessing           = "71"
	RCExceedsMaxPayment    = "72"
	RCRoutingError         = "73"
	RCAlreadySettled       = "74"
	RCMinInterval          = "75"
	RCExpiredNumber        = "78"
	RCExceedsMaxTunggakan  = "79"
	RCNoDataOrPaid         = "80"
	RCBlocked              = "83"
	RCCutOff               = "85"
	RCDuplicateRef         = "86"
)

// successCodes are RC values that indicate success
var successCodes = map[string]bool{
	RCSuccess: true,
}

// pendingCodes are RC values that indicate pending/processing (for Payment)
var pendingCodes = map[string]bool{
	RCBillerNoResponse:     true,
	RCNoResponseFromBiller: true,
	RCNoResponseFromHost:   true,
	RCStorageIssue:         true,
	RCBillerLinkDown:       true,
	RCTimeout:              true,
	RCProcessing:           true,
}

// fatalCodes are RC values that indicate definite failure (no retry)
var fatalCodes = map[string]bool{
	RCUndefinedError:      true, // Error tidak terdefinisi - should fail immediately
	RCFormatError:         true,
	RCCannotProcess:       true,
	RCInsufficientBalance: true,
	RCUnknownProduct:      true,
	RCTransactionFailed:   true,
	RCNotRegistered:       true,
	RCDataNotFound:        true, // Data tidak ditemukan - should fail for inquiry
	RCCannotTransact:      true,
	RCAdminError:          true,
	RCUnknownMessage:      true,
	RCNotAuthorized:       true,
	RCInvalidPrice:        true,
	RCExpired:             true,
	RCPayAtOffice:         true,
	RCInquiryRequired:     true,
	RCBillNotAvailable:    true,
	RCAlreadyPaid:         true,
	RCInvalidCustomer:     true,
	RCProcessFailure:      true, // Gagal proses - should fail immediately
	RCDailyLimitReached:   true,
	RCNumberNotAllowed:    true,
	RCInvalidAmount:       true,
	RCExceedsMaxPayment:   true,
	RCRoutingError:        true,
	RCAlreadySettled:      true,
	RCMinInterval:         true,
	RCExpiredNumber:       true,
	RCExceedsMaxTunggakan: true,
	RCNoDataOrPaid:        true,
	RCBlocked:             true,
	RCCutOff:              true,
	RCDuplicateRef:        true,
}

// sessionExpiredCodes indicate session needs refresh
var sessionExpiredCodes = map[string]bool{
	RCSessionExpired: true,
}

// IsSuccess returns true if RC indicates success
func IsSuccess(rc string) bool {
	return successCodes[rc]
}

// IsPending returns true if RC indicates pending (need to check status later)
func IsPending(rc string) bool {
	return pendingCodes[rc]
}

// IsFatal returns true if RC indicates definite failure
func IsFatal(rc string) bool {
	return fatalCodes[rc]
}

// IsSessionExpired returns true if session needs to be refreshed
func IsSessionExpired(rc string) bool {
	return sessionExpiredCodes[rc]
}

// NeedsNewRefID returns true if a new reference ID is required
func NeedsNewRefID(rc string) bool {
	return rc == RCDuplicateRef
}

// GetRCDescription returns human-readable description
func GetRCDescription(rc string) string {
	descriptions := map[string]string{
		RCSuccess:              "Transaksi berhasil",
		RCUndefinedError:       "Error tidak terdefinisi",
		RCFormatError:          "Format message salah",
		RCBillerNoResponse:     "Tidak diresponse oleh biller",
		RCNoResponseFromBiller: "No response from biller",
		RCNoResponseFromHost:   "No response from host",
		RCCannotProcess:        "Transaksi tidak dapat dilakukan",
		RCInsufficientBalance:  "Saldo tidak mencukupi",
		RCStorageIssue:         "Kendala media penyimpanan",
		RCUnknownProduct:       "Produk tidak dikenal",
		RCTransactionFailed:    "Transaksi gagal",
		RCNotRegistered:        "Switching hulu belum registrasi",
		RCDataNotFound:         "Data tidak ditemukan",
		RCCannotTransact:       "Transaksi tidak dapat dilakukan",
		RCSessionExpired:       "Session expired",
		RCAdminError:           "Error setting admin bank",
		RCBillerLinkDown:       "Link ke biller terputus",
		RCTimeout:              "Timeout from biller",
		RCUnknownMessage:       "Jenis message tidak dikenal",
		RCNotAuthorized:        "Tidak diperbolehkan transaksi produk ini",
		RCInvalidPrice:         "Harga jual tidak valid",
		RCExpired:              "Transaksi sudah kadaluwarsa",
		RCPayAtOffice:          "Bayar di kantor PDAM",
		RCInquiryRequired:      "Harus inquiry terlebih dahulu",
		RCBillNotAvailable:     "Tagihan baru belum tersedia",
		RCAlreadyPaid:          "Tagihan sudah terbayar",
		RCInvalidCustomer:      "Nomor pelanggan salah",
		RCProcessFailure:       "Kegagalan saat proses transaksi",
		RCDailyLimitReached:    "Limit transaksi per hari tercapai",
		RCNumberNotAllowed:     "Nomor tidak diperbolehkan transaksi",
		RCInvalidAmount:        "Nominal pembayaran salah",
		RCProcessing:           "Transaksi sedang diproses",
		RCExceedsMaxPayment:    "Pembayaran melebihi batas maksimal",
		RCRoutingError:         "Kegagalan routing transaksi",
		RCAlreadySettled:       "Transaksi sudah lunas",
		RCMinInterval:          "Jeda minimal 10 menit untuk nominal sama",
		RCExpiredNumber:        "Nomor sudah kadaluwarsa",
		RCExceedsMaxTunggakan:  "Tunggakan melebihi maksimal",
		RCNoDataOrPaid:         "Data tidak tersedia atau sudah lunas",
		RCBlocked:              "Nomor sedang diblokir",
		RCCutOff:               "Sedang dalam masa cut off",
		RCDuplicateRef:         "Kode referensi sudah digunakan",
	}
	if desc, ok := descriptions[rc]; ok {
		return desc
	}
	return "Unknown error"
}
