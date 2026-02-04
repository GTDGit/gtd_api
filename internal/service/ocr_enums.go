package service

import (
	"strings"
)

// Gender mapping from Indonesian to English enum
var genderMap = map[string]string{
	"LAKI-LAKI": "MAN",
	"LAKI LAKI": "MAN",
	"LAKILAKI":  "MAN",
	"PRIA":      "MAN",
	"L":         "MAN",
	"PEREMPUAN": "WOMAN",
	"WANITA":    "WOMAN",
	"P":         "WOMAN",
}

// Religion mapping from Indonesian to English enum
var religionMap = map[string]string{
	"ISLAM":             "ISLAM",
	"KRISTEN":           "PROTESTANT",
	"KRISTEN PROTESTAN": "PROTESTANT",
	"PROTESTAN":         "PROTESTANT",
	"KATOLIK":           "CATHOLIC",
	"KRISTEN KATOLIK":   "CATHOLIC",
	"HINDU":             "HINDU",
	"BUDDHA":            "BUDDHA",
	"BUDHA":             "BUDDHA",
	"KONGHUCU":          "KONGHUCU",
	"KHONGHUCU":         "KONGHUCU",
	"KEPERCAYAAN":       "BELIEF",
}

// MaritalStatus mapping from Indonesian to English enum
var maritalStatusMap = map[string]string{
	"BELUM KAWIN":          "UNMARRIED",
	"BELUMKAWIN":           "UNMARRIED",
	"BELUM MENIKAH":        "UNMARRIED",
	"KAWIN":                "MARRIED",
	"MENIKAH":              "MARRIED",
	"SUDAH KAWIN":          "MARRIED",
	"CERAI HIDUP":          "LIFE_DIVORCE",
	"CERAIHIDUP":           "LIFE_DIVORCE",
	"CERAI MATI":           "DEATH_DIVORCE",
	"CERAIMATI":            "DEATH_DIVORCE",
	"KAWIN BELUM TERCATAT": "UNREGISTERED_MARRIED",
	"KAWIN TERCATAT":       "MARRIED",
}

// Occupation mapping from Indonesian to English enum
var occupationMap = map[string]string{
	"BELUM/TIDAK BEKERJA":     "UNEMPLOYED",
	"BELUM TIDAK BEKERJA":     "UNEMPLOYED",
	"BELUMTIDAK BEKERJA":      "UNEMPLOYED",
	"TIDAK BEKERJA":           "UNEMPLOYED",
	"BELUM BEKERJA":           "UNEMPLOYED",
	"MENGURUS RUMAH TANGGA":   "HOUSEWIFE",
	"MENGURUSRUMAHTANGGA":     "HOUSEWIFE",
	"IBU RUMAH TANGGA":        "HOUSEWIFE",
	"PELAJAR/MAHASISWA":       "STUDENT",
	"PELAJAR MAHASISWA":       "STUDENT",
	"PELAJARMAHASISWA":        "STUDENT",
	"PELAJAR":                 "STUDENT",
	"MAHASISWA":               "STUDENT",
	"PENSIUNAN":               "RETIRED",
	"PENSIUN":                 "RETIRED",
	"WIRASWASTA":              "ENTREPRENEUR",
	"WIRAUSAHA":               "ENTREPRENEUR",
	"SWASTA":                  "PRIVATE_SECTOR",
	"PEGAWAI SWASTA":          "PRIVATE_SECTOR",
	"PNS":                     "CIVIL_SERVANT",
	"PEGAWAI NEGERI SIPIL":    "CIVIL_SERVANT",
	"PEGAWAI NEGERI":          "CIVIL_SERVANT",
	"TNI":                     "MILITARY",
	"TENTARA":                 "MILITARY",
	"POLRI":                   "POLICE",
	"POLISI":                  "POLICE",
	"KARYAWAN SWASTA":         "PRIVATE_EMPLOYEE",
	"KARYAWANSWASTA":          "PRIVATE_EMPLOYEE",
	"KARYAWAN BUMN":           "STATE_OWNED_EMPLOYEE",
	"PEGAWAI BUMN":            "STATE_OWNED_EMPLOYEE",
	"KARYAWAN BUMD":           "REGIONAL_OWNED_EMPLOYEE",
	"PEGAWAI BUMD":            "REGIONAL_OWNED_EMPLOYEE",
	"KARYAWAN HONORER":        "CONTRACT_EMPLOYEE",
	"HONORER":                 "CONTRACT_EMPLOYEE",
	"BURUH HARIAN LEPAS":      "DAILY_LABORER",
	"BURUH HARIAN":            "DAILY_LABORER",
	"BURUH":                   "DAILY_LABORER",
	"BURUH TANI/PERKEBUNAN":   "FARM_LABORER",
	"BURUH TANI":              "FARM_LABORER",
	"BURUH PERKEBUNAN":        "FARM_LABORER",
	"BURUH NELAYAN/PERIKANAN": "FISHERY_LABORER",
	"BURUH NELAYAN":           "FISHERY_LABORER",
	"BURUH PERIKANAN":         "FISHERY_LABORER",
	"BURUH PETERNAKAN":        "LIVESTOCK_LABORER",
	"PEMBANTU RUMAH TANGGA":   "DOMESTIC_HELPER",
	"PRT":                     "DOMESTIC_HELPER",
	"TUKANG CUKUR":            "BARBER",
	"TUKANG LISTRIK":          "ELECTRICIAN",
	"TUKANG BATU":             "MASON",
	"TUKANG KAYU":             "CARPENTER",
	"TUKANG SOL SEPATU":       "SHOE_REPAIRMAN",
	"TUKANG LAS/PANDAI BESI":  "WELDER",
	"TUKANG LAS":              "WELDER",
	"PANDAI BESI":             "WELDER",
	"TUKANG JAHIT":            "TAILOR",
	"PENJAHIT":                "TAILOR",
	"TUKANG GIGI":             "DENTAL_TECHNICIAN",
	"PENATA RAMBUT":           "HAIRDRESSER",
	"PENATA RIAS":             "MAKEUP_ARTIST",
	"MEKANIK":                 "MECHANIC",
	"SOPIR":                   "DRIVER",
	"SUPIR":                   "DRIVER",
	"DRIVER":                  "DRIVER",
	"MONTIR":                  "AUTO_MECHANIC",
	"PEDAGANG":                "TRADER",
	"PETANI":                  "FARMER",
	"PETERNAK":                "BREEDER",
	"NELAYAN":                 "FISHERMAN",
	"INDUSTRI RUMAH TANGGA":   "HOME_INDUSTRY",
	"PENGRAJIN":               "CRAFTSMAN",
	"SENIMAN":                 "ARTIST",
	"WARTAWAN":                "JOURNALIST",
	"USTADZ/MUBALIGH":         "ISLAMIC_PREACHER",
	"USTADZ":                  "ISLAMIC_PREACHER",
	"MUBALIGH":                "ISLAMIC_PREACHER",
	"JURU MASAK":              "CHEF",
	"KOKI":                    "CHEF",
	"PENJAGA TOKO":            "SHOPKEEPER",
	"PENJAGA WARUNG":          "FOOD_STALL_KEEPER",
	"PENDETA":                 "PASTOR",
	"PASTUR":                  "PRIEST",
	"PERAWAT":                 "NURSE",
	"BIDAN":                   "MIDWIFE",
	"DOKTER":                  "DOCTOR",
	"APOTEKER":                "PHARMACIST",
	"PSIKIATER":               "PSYCHIATRIST",
	"PSIKOLOG":                "PSYCHOLOGIST",
	"GURU":                    "TEACHER",
	"DOSEN":                   "LECTURER",
	"TENAGA PENGAJAR":         "EDUCATOR",
	"KONSULTAN":               "CONSULTANT",
	"NOTARIS":                 "NOTARY",
	"PENGACARA":               "LAWYER",
	"ADVOKAT":                 "LAWYER",
	"HAKIM":                   "JUDGE",
	"JAKSA":                   "PROSECUTOR",
	"ARSITEK":                 "ARCHITECT",
	"AKUNTAN":                 "ACCOUNTANT",
	"DESAINER":                "DESIGNER",
	"MANAJER":                 "MANAGER",
	"MANAGER":                 "MANAGER",
	"DIREKTUR":                "DIRECTOR",
	"PROGRAMMER":              "PROGRAMMER",
	"ANALIS SISTEM":           "SYSTEM_ANALYST",
	"TEKNISI":                 "TECHNICIAN",
	"WAKIL GUBERNUR":          "VICE_GOVERNOR",
	"GUBERNUR":                "GOVERNOR",
	"WAKIL BUPATI":            "VICE_REGENT",
	"BUPATI":                  "REGENT",
	"ANGGOTA DPRD PROVINSI":   "PROVINCIAL_COUNCIL_MEMBER",
	"ANGGOTA DPRD KABUPATEN":  "REGIONAL_COUNCIL_MEMBER",
	"ANGGOTA DPRD":            "REGIONAL_COUNCIL_MEMBER",
	"TENAGA TATA USAHA":       "ADMINISTRATIVE_STAFF",
	"EDITOR":                  "EDITOR",
	"PENULIS":                 "WRITER",
	"PENYIAR":                 "BROADCASTER",
	"PRESENTER":               "PRESENTER",
	"PENELITI":                "RESEARCHER",
	"FOTOGRAFER":              "PHOTOGRAPHER",
	"VIDEOGRAFER":             "VIDEOGRAPHER",
	"DUTA BESAR":              "AMBASSADOR",
	"WAITER/WAITRESS":         "WAITER",
	"WAITER":                  "WAITER",
	"WAITRESS":                "WAITER",
	"BIARAWATI":               "NUN",
	"SATPAM":                  "SECURITY_GUARD",
	"SECURITY":                "SECURITY_GUARD",
	"PETUGAS KEBERSIHAN":      "JANITOR",
	"CLEANING SERVICE":        "JANITOR",
	"PRAMUGARI/PRAMUGARA":     "FLIGHT_ATTENDANT",
	"PRAMUGARI":               "FLIGHT_ATTENDANT",
	"PRAMUGARA":               "FLIGHT_ATTENDANT",
	"MASINIS":                 "TRAIN_DRIVER",
	"PILOT":                   "PILOT",
	"NAHKODA":                 "SHIP_CAPTAIN",
	"ABK":                     "SHIP_CREW",
	"ANAK BUAH KAPAL":         "SHIP_CREW",
	"PENERJEMAH":              "TRANSLATOR",
	"PEMANDU WISATA":          "TOUR_GUIDE",
	"TRAVEL AGENT":            "TRAVEL_AGENT",
	"PEGAWAI BANK":            "BANK_EMPLOYEE",
	"PEGAWAI ASURANSI":        "INSURANCE_EMPLOYEE",
	"PEGAWAI PAJAK":           "TAX_EMPLOYEE",
	"PEGAWAI BEA CUKAI":       "CUSTOMS_EMPLOYEE",
	"PEGAWAI IMIGRASI":        "IMMIGRATION_EMPLOYEE",
	"ASISTEN AHLI":            "ASSISTANT_EXPERT",
	"PROMOTOR ACARA":          "EVENT_PROMOTER",
	"INVESTOR":                "INVESTOR",
	"LAINNYA":                 "OTHER",
}

// Nationality mapping
var nationalityMap = map[string]string{
	"WNI":       "WNI",
	"INDONESIA": "WNI",
	"WNA":       "WNA",
}

// BloodType valid values
var bloodTypeMap = map[string]string{
	"A":  "A",
	"B":  "B",
	"AB": "AB",
	"O":  "O",
}

// MapGender maps Indonesian gender to English enum
func MapGender(indonesian string) string {
	normalized := strings.ToUpper(strings.TrimSpace(indonesian))
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.ReplaceAll(normalized, "  ", " ")

	if val, ok := genderMap[normalized]; ok {
		return val
	}
	// Fuzzy match
	for key, val := range genderMap {
		if strings.Contains(normalized, key) || strings.Contains(key, normalized) {
			return val
		}
	}
	return "MAN" // default
}

// MapReligion maps Indonesian religion to English enum
func MapReligion(indonesian string) string {
	normalized := strings.ToUpper(strings.TrimSpace(indonesian))

	if val, ok := religionMap[normalized]; ok {
		return val
	}
	// Fuzzy match
	for key, val := range religionMap {
		if strings.Contains(normalized, key) || strings.Contains(key, normalized) {
			return val
		}
	}
	return "ISLAM" // default (most common in Indonesia)
}

// MapMaritalStatus maps Indonesian marital status to English enum
func MapMaritalStatus(indonesian string) string {
	normalized := strings.ToUpper(strings.TrimSpace(indonesian))
	normalized = strings.ReplaceAll(normalized, " ", "")

	// Try exact match first
	if val, ok := maritalStatusMap[normalized]; ok {
		return val
	}

	// Try with spaces
	normalized = strings.ToUpper(strings.TrimSpace(indonesian))
	if val, ok := maritalStatusMap[normalized]; ok {
		return val
	}

	// Fuzzy match
	for key, val := range maritalStatusMap {
		keyNorm := strings.ReplaceAll(key, " ", "")
		if strings.Contains(normalized, keyNorm) || strings.Contains(keyNorm, normalized) {
			return val
		}
	}
	return "UNMARRIED" // default
}

// MapOccupation maps Indonesian occupation to English enum
func MapOccupation(indonesian string) string {
	normalized := strings.ToUpper(strings.TrimSpace(indonesian))

	// Try exact match
	if val, ok := occupationMap[normalized]; ok {
		return val
	}

	// Remove spaces and try
	noSpace := strings.ReplaceAll(normalized, " ", "")
	if val, ok := occupationMap[noSpace]; ok {
		return val
	}

	// Fuzzy match - check if any key is contained in input
	for key, val := range occupationMap {
		keyNoSpace := strings.ReplaceAll(key, " ", "")
		if strings.Contains(noSpace, keyNoSpace) || strings.Contains(keyNoSpace, noSpace) {
			return val
		}
		if strings.Contains(normalized, key) {
			return val
		}
	}

	// Check partial matches
	if strings.Contains(normalized, "PELAJAR") || strings.Contains(normalized, "MAHASISWA") {
		return "STUDENT"
	}
	if strings.Contains(normalized, "RUMAH TANGGA") || strings.Contains(normalized, "RUMAHTANGGA") {
		return "HOUSEWIFE"
	}
	if strings.Contains(normalized, "TIDAK BEKERJA") || strings.Contains(normalized, "BELUM") {
		return "UNEMPLOYED"
	}
	if strings.Contains(normalized, "WIRASWASTA") || strings.Contains(normalized, "WIRAUSAHA") {
		return "ENTREPRENEUR"
	}
	if strings.Contains(normalized, "KARYAWAN") {
		return "PRIVATE_EMPLOYEE"
	}
	if strings.Contains(normalized, "PEGAWAI") {
		return "PRIVATE_EMPLOYEE"
	}

	return "OTHER" // default
}

// MapNationality maps Indonesian nationality to English enum
func MapNationality(indonesian string) string {
	normalized := strings.ToUpper(strings.TrimSpace(indonesian))

	if val, ok := nationalityMap[normalized]; ok {
		return val
	}

	if strings.Contains(normalized, "WNI") || strings.Contains(normalized, "INDONESIA") {
		return "WNI"
	}

	return "WNI" // default for Indonesian KTP
}

// MapBloodType maps blood type, returns empty string if invalid
func MapBloodType(bloodType string) string {
	if bloodType == "" {
		return ""
	}
	normalized := strings.ToUpper(strings.TrimSpace(bloodType))

	if val, ok := bloodTypeMap[normalized]; ok {
		return val
	}

	// Check if contains valid blood type
	for key, val := range bloodTypeMap {
		if strings.Contains(normalized, key) {
			return val
		}
	}

	return "" // invalid blood type
}

// MapValidUntil maps validUntil field
func MapValidUntil(validUntil string) string {
	normalized := strings.ToUpper(strings.TrimSpace(validUntil))

	if strings.Contains(normalized, "SEUMUR HIDUP") || strings.Contains(normalized, "SEUMUR") || strings.Contains(normalized, "LIFETIME") {
		return "LIFETIME"
	}

	// Return as-is if it looks like a date
	return validUntil
}

// NormalizeProvince removes PROVINSI prefix
func NormalizeProvince(province string) string {
	normalized := strings.ToUpper(strings.TrimSpace(province))
	normalized = strings.TrimPrefix(normalized, "PROVINSI ")
	normalized = strings.TrimPrefix(normalized, "PROV. ")
	normalized = strings.TrimPrefix(normalized, "PROV ")
	return strings.TrimSpace(normalized)
}

// NormalizeCity removes KOTA/KABUPATEN prefix for matching but keeps for display
func NormalizeCity(city string) string {
	normalized := strings.ToUpper(strings.TrimSpace(city))
	return normalized
}

// NormalizeCityForMatching removes KOTA/KABUPATEN prefix for database matching
func NormalizeCityForMatching(city string) string {
	normalized := strings.ToUpper(strings.TrimSpace(city))
	normalized = strings.TrimPrefix(normalized, "KOTA ADMINISTRASI ")
	normalized = strings.TrimPrefix(normalized, "KOTA ADM. ")
	normalized = strings.TrimPrefix(normalized, "KOTA ")
	normalized = strings.TrimPrefix(normalized, "KABUPATEN ")
	normalized = strings.TrimPrefix(normalized, "KAB. ")
	normalized = strings.TrimPrefix(normalized, "KAB ")
	return strings.TrimSpace(normalized)
}

// NormalizeDistrict removes KECAMATAN prefix
func NormalizeDistrict(district string) string {
	normalized := strings.ToUpper(strings.TrimSpace(district))
	normalized = strings.TrimPrefix(normalized, "KECAMATAN ")
	normalized = strings.TrimPrefix(normalized, "KEC. ")
	normalized = strings.TrimPrefix(normalized, "KEC ")
	return strings.TrimSpace(normalized)
}

// NormalizeSubDistrict removes KELURAHAN/DESA prefix
func NormalizeSubDistrict(subDistrict string) string {
	normalized := strings.ToUpper(strings.TrimSpace(subDistrict))
	normalized = strings.TrimPrefix(normalized, "KELURAHAN ")
	normalized = strings.TrimPrefix(normalized, "KEL. ")
	normalized = strings.TrimPrefix(normalized, "KEL ")
	normalized = strings.TrimPrefix(normalized, "DESA ")
	return strings.TrimSpace(normalized)
}
