"""
Parse Alterra product catalog Excel and generate SQL migration.

Usage:
  python scripts/parse_alterra_catalog.py

Reads: docs/ppob/alterra/20262801 - BPA Product Catalogue  (OPEN).xlsx
Writes: migrations/000026_overhaul_products_proper.up.sql
        migrations/000026_overhaul_products_proper.down.sql

Categories: Pulsa, Listrik, BPJS Kesehatan, PDAM, Gas PGN, Telepon Pascabayar,
            Internet, TV Kabel, Streaming, Voucher Game, E-Money, Voucher, Edukasi, PBB, Donasi
"""

import openpyxl
import os
import re

EXCEL_PATH = os.path.join(os.path.dirname(__file__), "..", "docs", "ppob", "alterra",
                          "20262801 - BPA Product Catalogue  (OPEN).xlsx")
UP_PATH = os.path.join(os.path.dirname(__file__), "..", "migrations", "000026_overhaul_products_proper.up.sql")
DOWN_PATH = os.path.join(os.path.dirname(__file__), "..", "migrations", "000026_overhaul_products_proper.down.sql")


def escape_sql(s):
    if s is None:
        return ""
    return str(s).replace("'", "''").strip()


def format_rupiah(amount):
    """Format number as Indonesian Rupiah string: 5000 -> '5.000', 1000000 -> '1.000.000'"""
    s = str(int(amount))
    result = []
    for i, c in enumerate(reversed(s)):
        if i > 0 and i % 3 == 0:
            result.append(".")
        result.append(c)
    return "".join(reversed(result))


def normalize_brand(operator):
    """Normalize operator name to standard brand."""
    if not operator:
        return "LAINNYA"
    op = str(operator).strip().lower()

    # Mobile operators
    if "axis" in op:
        return "AXIS"
    if "xl" in op and "axis" not in op:
        return "XL"
    if "telkomsel" in op:
        return "TELKOMSEL"
    if "indosat" in op or "im3" in op or "ooredoo" in op:
        return "INDOSAT"
    if "tri" in op or "three" in op:
        return "TRI"
    if "smartfren" in op:
        return "SMARTFREN"
    if "by.u" in op or "byu" in op:
        return "BYU"

    # Telco
    if op == "pln":
        return "PLN"
    if "telkom" in op:
        return "TELKOM"

    # E-wallet brands
    if "dana" in op:
        return "DANA"
    if "gopay" in op:
        return "GOPAY"
    if "ovo" in op:
        return "OVO"
    if "shopee" in op:
        return "SHOPEEPAY"
    if "linkaja" in op:
        return "LINKAJA"
    if "mandiri" in op or "e-money" in op:
        return "MANDIRI E-MONEY"
    if "bni" in op or "tapcash" in op:
        return "BNI TAPCASH"
    if "brizzi" in op or "bri" in op:
        return "BRIZZI"
    if "maxim" in op:
        return "MAXIM"

    # Clean up generic
    brand = str(operator).strip().upper()
    brand = re.sub(r'[^A-Z0-9 &\-]', '', brand)
    return brand if brand else "LAINNYA"


def safe_int(val):
    """Safely convert value to int."""
    if val is None:
        return 0
    if isinstance(val, (int, float)):
        return int(val)
    try:
        return int(str(val).replace(",", "").replace(".", "").strip())
    except (ValueError, TypeError):
        return 0


def is_product_id(val):
    """Check if value looks like a product ID (integer)."""
    return isinstance(val, (int, float)) and val is not None and val > 0


# ============================================================
# Sheet parsers
# ============================================================

def parse_pulsa(ws):
    """Parse Pulsa sheet - prepaid mobile top-up."""
    products = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        vals = list(row)[:7]
        product_id = vals[0]
        if not is_product_id(product_id):
            continue

        product_id = int(product_id)
        name = str(vals[2]).strip() if vals[2] else ""
        denom = safe_int(vals[3])
        operator = str(vals[4]).strip() if vals[4] else ""
        sell_price = safe_int(vals[6])

        brand = normalize_brand(operator)
        display_name = f"Rp {format_rupiah(denom)}" if denom > 0 else name
        sku_code = f"ALT-PULSA-{brand}-{denom}" if denom > 0 else f"ALT-PULSA-{brand}-{product_id}"

        products.append({
            "sku_code": sku_code,
            "name": display_name,
            "category": "Pulsa",
            "brand": brand,
            "type": "prepaid",
            "description": name,
            "alterra_product_id": product_id,
            "price": sell_price,
        })
    return products


def parse_pln_bpjs_postpaid(ws):
    """Parse 'PLN, BPJS & Postpaid' sheet - multiple sections."""
    products = []
    current_section = None

    for row in ws.iter_rows(values_only=True):
        vals = list(row)[:8]
        first_val = vals[0]

        # Detect section headers
        if isinstance(first_val, str):
            lower = first_val.lower().strip()
            if "pln prepaid" in lower:
                current_section = "pln_prepaid"
                continue
            elif "pln postpaid" in lower:
                current_section = "pln_postpaid"
                continue
            elif "bpjs" in lower and ("product" in lower or "produk" in lower):
                current_section = "bpjs"
                continue
            elif "telkom postpaid" in lower:
                current_section = "telkom_postpaid"
                continue
            elif "mobile postpaid" in lower:
                current_section = "mobile_postpaid"
                continue
            elif "internet service" in lower:
                current_section = "internet_service"
                continue
            elif "insurance" in lower:
                current_section = "insurance"
                continue
            if "product id" in lower:
                continue

        if not is_product_id(first_val):
            continue

        product_id = int(first_val)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""
        operator = str(vals[3]).strip() if vals[3] else ""
        service_fee = safe_int(vals[4])

        if current_section == "insurance":
            continue

        if current_section == "pln_prepaid":
            # Extract denomination from name
            denom = 0
            m = re.search(r'[\d,]+', name.replace("PLN Prepaid Rp. ", "").replace("PLN Prepaid Rp.", ""))
            if m:
                denom = safe_int(m.group(0).replace(",", ""))
            display_name = f"Token Rp {format_rupiah(denom)}" if denom > 0 else name
            sku_code = f"ALT-LISTRIK-PLN-{denom}" if denom > 0 else f"ALT-LISTRIK-PLN-{product_id}"
            products.append({
                "sku_code": sku_code, "name": display_name,
                "category": "Listrik", "brand": "PLN", "type": "prepaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif current_section == "pln_postpaid":
            products.append({
                "sku_code": f"ALT-LISTRIK-PLN-POSTPAID-{product_id}",
                "name": "Tagihan Listrik",
                "category": "Listrik", "brand": "PLN", "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif current_section == "bpjs":
            products.append({
                "sku_code": f"ALT-BPJS-{product_id}",
                "name": "BPJS Kesehatan",
                "category": "BPJS Kesehatan", "brand": "BPJS", "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif current_section == "telkom_postpaid":
            products.append({
                "sku_code": f"ALT-TELPASCABAYAR-TELKOM-{product_id}",
                "name": name if name else "Tagihan Telkom",
                "category": "Telepon Pascabayar", "brand": "TELKOM", "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif current_section == "mobile_postpaid":
            brand = normalize_brand(operator)
            products.append({
                "sku_code": f"ALT-TELPASCABAYAR-{brand}-{product_id}",
                "name": name if name else f"Pascabayar {brand}",
                "category": "Telepon Pascabayar", "brand": brand, "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif current_section == "internet_service":
            brand = normalize_brand(operator)
            products.append({
                "sku_code": f"ALT-INTERNET-{brand}-{product_id}",
                "name": name if name else f"Internet {brand}",
                "category": "Internet", "brand": brand, "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

    return products


def parse_pbb(ws):
    """Parse PBB sheet - property tax."""
    products = []
    for row in ws.iter_rows(values_only=True):
        vals = list(row)[:7]
        first_val = vals[0]

        if isinstance(first_val, str) and ("product" in first_val.lower() or "direct" in first_val.lower()
                                           or "produk" in first_val.lower()):
            continue
        if not is_product_id(first_val):
            continue

        product_id = int(first_val)
        name = str(vals[2]).strip() if vals[2] else ""
        service_fee = safe_int(vals[4])
        bpd = str(vals[5]).strip() if vals[5] else ""

        # Clean name
        clean_name = name.replace("\t", "").strip()
        brand = "PBB"

        products.append({
            "sku_code": f"ALT-PBB-{product_id}",
            "name": clean_name if clean_name else f"PBB {bpd}",
            "category": "PBB", "brand": brand, "type": "postpaid",
            "description": f"{clean_name} - {bpd}" if bpd else clean_name,
            "alterra_product_id": product_id, "price": service_fee,
        })
    return products


def parse_gas_pdam(ws):
    """Parse Gas & PDAM sheet."""
    products = []
    current_section = None

    for row in ws.iter_rows(values_only=True):
        vals = list(row)[:8]
        first_val = vals[0]

        if isinstance(first_val, str):
            lower = first_val.lower().strip()
            if lower == "gas":
                current_section = "gas"
                continue
            elif lower == "pdam":
                current_section = "pdam"
                continue
            if "product" in lower:
                continue

        if not is_product_id(first_val):
            continue

        product_id = int(first_val)
        name = str(vals[2]).strip() if vals[2] else ""
        operator = str(vals[4]).strip() if vals[4] else name
        service_fee = safe_int(vals[5])

        if current_section == "gas":
            brand = normalize_brand(operator) if operator else "PGN"
            products.append({
                "sku_code": f"ALT-GAS-{product_id}",
                "name": name,
                "category": "Gas PGN", "brand": brand, "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif current_section == "pdam":
            # Check status column if available
            status = str(vals[6]).strip().upper() if vals[6] else "OPEN"
            if status != "OPEN" and status != "":
                continue

            # Extract PDAM name as brand
            pdam_name = name.upper().replace("PDAM ", "").strip()
            brand = f"PDAM {pdam_name}" if not name.upper().startswith("PDAM") else name.upper()
            # Simplify brand
            brand = name.upper() if len(name) < 40 else pdam_name[:30]

            products.append({
                "sku_code": f"ALT-PDAM-{product_id}",
                "name": name,
                "category": "PDAM", "brand": "PDAM", "type": "postpaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

    return products


def parse_ticket_ewallet(ws):
    """Parse Ticket & Ewallet sheet - use product type column to categorize."""
    products = []
    col_has_denom = False
    col_has_type = False

    for i, row in enumerate(ws.iter_rows(values_only=True)):
        vals = list(row)[:8]
        first_val = vals[0]

        # Detect column format from header rows
        if isinstance(first_val, str):
            lower = first_val.lower().strip()
            if "product id" in lower or "product id" == lower:
                # Check what columns we have
                headers = [str(v).lower().strip() if v else "" for v in vals]
                col_has_type = any("tipe" in h or "type" in h for h in headers[1:3])
                col_has_denom = any("denom" in h for h in headers)
                continue
            continue

        if not is_product_id(first_val):
            continue

        product_id = int(first_val)

        # Determine product type from column 1 (if type column exists)
        tipe = str(vals[1]).strip().lower() if vals[1] and col_has_type else ""

        # Route based on product type value
        if "ticket" in tipe or "train" in tipe:
            name = str(vals[2]).strip() if vals[2] else ""
            operator = str(vals[4]).strip() if vals[4] else ""
            service_fee = safe_int(vals[5])
            brand = normalize_brand(operator) if operator else "LAINNYA"
            products.append({
                "sku_code": f"ALT-TIKET-{product_id}",
                "name": name,
                "category": "Tiket", "brand": brand, "type": "prepaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif "ewallet" in tipe or "wallet" in tipe:
            # E-wallet product - cols vary but typically: ID, Tipe, Produk, Denom, Operator, Price
            name = str(vals[2]).strip() if vals[2] else ""
            denom = safe_int(vals[3])
            operator = str(vals[4]).strip() if vals[4] else ""
            service_fee = safe_int(vals[5])
            # Some rows have operator in col 3 instead of denom
            if denom == 0 and isinstance(vals[3], str):
                operator = str(vals[3]).strip()
                service_fee = safe_int(vals[4])

            brand = normalize_brand(operator) if operator else normalize_brand(name)
            display_name = f"Rp {format_rupiah(denom)}" if denom > 0 else name

            products.append({
                "sku_code": f"ALT-EMONEY-{brand}-{product_id}",
                "name": display_name,
                "category": "E-Money", "brand": brand, "type": "prepaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        elif not col_has_type:
            # No type column - likely e-meterai or similar (ID, Produk, Fee)
            name = str(vals[1]).strip() if vals[1] else ""
            service_fee = safe_int(vals[2])
            brand = "E-METERAI" if "meterai" in name.lower() else "LAINNYA"
            products.append({
                "sku_code": f"ALT-EMONEY-{product_id}",
                "name": name,
                "category": "E-Money", "brand": brand, "type": "prepaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

        else:
            # Unknown type - still add as e-money
            name = str(vals[2]).strip() if vals[2] else ""
            denom = safe_int(vals[3])
            operator = str(vals[4]).strip() if vals[4] else ""
            service_fee = safe_int(vals[5])
            brand = normalize_brand(operator) if operator else normalize_brand(name)
            display_name = f"Rp {format_rupiah(denom)}" if denom > 0 else name
            products.append({
                "sku_code": f"ALT-EMONEY-{brand}-{product_id}",
                "name": display_name,
                "category": "E-Money", "brand": brand, "type": "prepaid",
                "description": name, "alterra_product_id": product_id, "price": service_fee,
            })

    return products


def parse_game(ws):
    """Parse Game sheet - game top-ups."""
    products = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        vals = list(row)[:7]
        product_id = vals[0]
        if not is_product_id(product_id):
            continue

        product_id = int(product_id)
        name = str(vals[2]).strip() if vals[2] else ""
        denom = safe_int(vals[3])
        publisher = str(vals[4]).strip() if vals[4] else ""
        service_fee = safe_int(vals[5])

        # Extract game name as brand
        brand = publisher.upper().replace("TOPUP ", "").replace("TOP UP ", "").strip()
        brand = re.sub(r'[^A-Z0-9 &\-]', '', brand).strip()
        if not brand:
            brand = "LAINNYA"

        products.append({
            "sku_code": f"ALT-GAME-{product_id}",
            "name": name,
            "category": "Voucher Game", "brand": brand, "type": "prepaid",
            "description": name, "alterra_product_id": product_id, "price": service_fee,
        })
    return products


def parse_streaming_tv(ws):
    """Parse Streaming & TV sheet."""
    products = []
    current_section = None

    for row in ws.iter_rows(values_only=True):
        vals = list(row)[:8]
        first_val = vals[0]

        if isinstance(first_val, str):
            lower = first_val.lower()
            if "streaming" in lower and ("voucher" in lower or "prepaid" in lower):
                current_section = "streaming"
                continue
            elif "tv" in lower and ("postpaid" in lower or "cable" in lower or "tagihan" in lower):
                current_section = "tv_postpaid"
                continue
            if "product" in lower:
                continue

        if not is_product_id(first_val):
            continue

        product_id = int(first_val)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""
        denom = safe_int(vals[3])
        publisher = str(vals[4]).strip() if vals[4] else ""
        service_fee = safe_int(vals[5])

        brand = publisher.upper().strip()
        brand = re.sub(r'[^A-Z0-9 &\-]', '', brand).strip() or "LAINNYA"

        tipe_lower = tipe.lower()
        if current_section == "tv_postpaid" or "postpaid" in tipe_lower or "tagihan" in tipe_lower or "cable" in tipe_lower:
            category = "TV Kabel"
            prod_type = "postpaid"
        else:
            category = "Streaming"
            prod_type = "prepaid"

        cat_prefix = "TVKABEL" if category == "TV Kabel" else "STREAMING"
        products.append({
            "sku_code": f"ALT-{cat_prefix}-{product_id}",
            "name": name,
            "category": category, "brand": brand, "type": prod_type,
            "description": name, "alterra_product_id": product_id, "price": service_fee,
        })
    return products


def parse_voucher_deals(ws):
    """Parse Voucher Deals sheet."""
    products = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        vals = list(row)[:7]
        product_id = vals[0]
        if not is_product_id(product_id):
            continue

        product_id = int(product_id)
        name = str(vals[2]).strip() if vals[2] else ""
        denom = safe_int(vals[3])
        operator = str(vals[4]).strip() if vals[4] else ""
        service_fee = safe_int(vals[5])

        brand = operator.upper().replace("VOUCHER ", "").strip()
        brand = re.sub(r'[^A-Z0-9 &\-]', '', brand).strip() or "LAINNYA"
        display_name = f"Rp {format_rupiah(denom)}" if denom > 0 else name

        products.append({
            "sku_code": f"ALT-VOUCHER-{product_id}",
            "name": display_name,
            "category": "Voucher", "brand": brand, "type": "prepaid",
            "description": name, "alterra_product_id": product_id, "price": service_fee,
        })
    return products


def parse_donation(ws):
    """Parse Donation sheet."""
    products = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        vals = list(row)[:7]
        product_id = vals[0]
        if not is_product_id(product_id):
            continue

        product_id = int(product_id)
        name = str(vals[2]).strip() if vals[2] else ""
        denom = safe_int(vals[3])
        operator = str(vals[4]).strip() if vals[4] else ""
        service_fee = safe_int(vals[5])

        brand = operator.upper().replace("INFAQ ", "").strip()
        brand = re.sub(r'[^A-Z0-9 &\-]', '', brand).strip() or "LAINNYA"
        display_name = f"Rp {format_rupiah(denom)}" if denom > 0 else name

        products.append({
            "sku_code": f"ALT-DONASI-{product_id}",
            "name": display_name,
            "category": "Donasi", "brand": brand, "type": "prepaid",
            "description": name, "alterra_product_id": product_id, "price": service_fee,
        })
    return products


def parse_edukasi(ws):
    """Parse Edukasi sheet - postpaid education payments."""
    products = []
    for row in ws.iter_rows(values_only=True):
        vals = list(row)[:7]
        product_id = vals[0]
        if not is_product_id(product_id):
            continue

        product_id = int(product_id)
        name = str(vals[2]).strip() if vals[2] else ""
        operator = str(vals[3]).strip() if vals[3] else ""
        service_fee = safe_int(vals[4])

        brand = operator.upper().strip() if operator else "LAINNYA"
        brand = re.sub(r'[^A-Z0-9 &\-]', '', brand).strip() or "LAINNYA"

        products.append({
            "sku_code": f"ALT-EDU-{product_id}",
            "name": name,
            "category": "Edukasi", "brand": brand, "type": "postpaid",
            "description": name, "alterra_product_id": product_id, "price": service_fee,
        })
    return products


def parse_voucher_edukasi(ws):
    """Parse Voucher Edukasi sheet - prepaid education vouchers."""
    products = []
    prev_product_id = None
    for row in ws.iter_rows(min_row=2, values_only=True):
        vals = list(row)[:7]
        raw_id = vals[0]

        # Handle Excel formula references like '=A2+1'
        if isinstance(raw_id, str) and raw_id.startswith("="):
            if prev_product_id is not None:
                raw_id = prev_product_id + 1
            else:
                continue
        if not is_product_id(raw_id):
            continue

        product_id = int(raw_id)
        prev_product_id = product_id
        name = str(vals[2]).strip() if vals[2] else ""
        denom = safe_int(vals[3])
        operator = str(vals[4]).strip() if vals[4] else ""
        service_fee = safe_int(vals[5])

        brand = operator.upper().strip() if operator else "LAINNYA"
        brand = re.sub(r'[^A-Z0-9 &\-]', '', brand).strip() or "LAINNYA"
        display_name = f"Rp {format_rupiah(denom)}" if denom > 0 else name

        products.append({
            "sku_code": f"ALT-VOUCHEREDU-{product_id}",
            "name": display_name,
            "category": "Edukasi", "brand": brand, "type": "prepaid",
            "description": name, "alterra_product_id": product_id, "price": service_fee,
        })
    return products


# ============================================================
# SQL Generation
# ============================================================

# Master categories with display order
CATEGORIES = [
    ("Pulsa", 1),
    ("Listrik", 2),
    ("BPJS Kesehatan", 3),
    ("PDAM", 4),
    ("Gas PGN", 5),
    ("Telepon Pascabayar", 6),
    ("Internet", 7),
    ("TV Kabel", 8),
    ("Streaming", 9),
    ("Voucher Game", 10),
    ("E-Money", 11),
    ("Voucher", 12),
    ("Donasi", 13),
    ("Edukasi", 14),
    ("PBB", 15),
    ("Tiket", 16),
]


def generate_sql(all_products):
    """Generate SQL migration from parsed products."""
    # Deduplicate by alterra_product_id
    seen = set()
    unique = []
    for p in all_products:
        if p["alterra_product_id"] not in seen:
            seen.add(p["alterra_product_id"])
            unique.append(p)

    # Deduplicate sku_codes
    sku_counts = {}
    for p in unique:
        sku = p["sku_code"]
        sku_counts[sku] = sku_counts.get(sku, 0) + 1

    sku_seen = {}
    for p in unique:
        sku = p["sku_code"]
        if sku_counts[sku] > 1:
            p["sku_code"] = f"{sku}-{p['alterra_product_id']}"
        if p["sku_code"] in sku_seen:
            p["sku_code"] = f"{p['sku_code']}-{p['alterra_product_id']}"
        sku_seen[p["sku_code"]] = True

    # Collect unique brands
    brands = {}
    for p in unique:
        b = p["brand"]
        if b not in brands:
            brands[b] = len(brands) + 1

    lines = []
    lines.append("-- ============================================")
    lines.append("-- Migration 000026: Complete Product Data Overhaul")
    lines.append("-- Auto-generated by scripts/parse_alterra_catalog.py")
    lines.append("-- Proper Indonesian PPOB standard categories, brands, and naming")
    lines.append("-- ============================================")
    lines.append("")
    lines.append("BEGIN;")
    lines.append("")

    # 1. Clean everything
    lines.append("-- 1. Clean all existing data")
    lines.append("DELETE FROM ppob_provider_skus;")
    lines.append("DELETE FROM skus;")
    lines.append("DELETE FROM products;")
    lines.append("DELETE FROM product_categories;")
    lines.append("DELETE FROM product_brands;")
    lines.append("")

    # 2. Seed master categories
    lines.append("-- 2. Seed master categories")
    cat_values = ", ".join(
        f"('{escape_sql(name)}', {order})"
        for name, order in CATEGORIES
    )
    lines.append(f"INSERT INTO product_categories (name, display_order) VALUES {cat_values}")
    lines.append("ON CONFLICT (name) DO UPDATE SET display_order = EXCLUDED.display_order;")
    lines.append("")

    # 3. Seed master brands
    lines.append("-- 3. Seed master brands")
    brand_values = ", ".join(
        f"('{escape_sql(b)}', {i})"
        for b, i in sorted(brands.items(), key=lambda x: x[1])
    )
    lines.append(f"INSERT INTO product_brands (name, display_order) VALUES {brand_values}")
    lines.append("ON CONFLICT (name) DO UPDATE SET display_order = EXCLUDED.display_order;")
    lines.append("")

    # 4. Insert products
    lines.append("-- 4. Insert products")
    for p in unique:
        lines.append(
            f"INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active) "
            f"VALUES ('{escape_sql(p['sku_code'])}', '{escape_sql(p['name'])}', '{escape_sql(p['category'])}', "
            f"'{escape_sql(p['brand'])}', '{p['type']}', 0, 0, "
            f"'{escape_sql(p['description'])}', true) "
            f"ON CONFLICT (sku_code) DO NOTHING;"
        )

    lines.append("")
    lines.append("-- 5. Insert provider SKU mappings for Alterra (dynamic provider_id lookup)")

    for p in unique:
        lines.append(
            f"INSERT INTO ppob_provider_skus (provider_id, product_id, provider_sku_code, provider_product_name, price, admin, commission, is_active, is_available) "
            f"SELECT (SELECT id FROM ppob_providers WHERE code = 'alterra'), p.id, '{p['alterra_product_id']}', '{escape_sql(p['description'])}', "
            f"{p['price']}, 0, 0, true, true "
            f"FROM products p WHERE p.sku_code = '{escape_sql(p['sku_code'])}' "
            f"ON CONFLICT (provider_id, provider_sku_code) DO UPDATE SET "
            f"price = EXCLUDED.price, provider_product_name = EXCLUDED.provider_product_name, "
            f"is_active = true, is_available = true;"
        )

    lines.append("")
    lines.append("-- 6. Ensure Alterra provider is active, Digiflazz disabled")
    lines.append("UPDATE ppob_providers SET is_active = true WHERE code = 'alterra';")
    lines.append("UPDATE ppob_providers SET is_active = false WHERE code = 'digiflazz';")
    lines.append("")
    lines.append("COMMIT;")

    return "\n".join(lines)


def generate_down_sql():
    return (
        "-- Rollback: restore Digiflazz, products need manual re-seed\n"
        "UPDATE ppob_providers SET is_active = true WHERE code = 'digiflazz';\n"
        "\n"
        "-- Note: Products are not restored. Run sync worker or re-apply previous migration.\n"
    )


def main():
    wb = openpyxl.load_workbook(EXCEL_PATH, read_only=True)

    all_products = []

    sheets_to_parse = [
        ("Pulsa", parse_pulsa),
        ("PLN, BPJS & Postpaid", parse_pln_bpjs_postpaid),
        ("PBB", parse_pbb),
        ("Gas & PDAM", parse_gas_pdam),
        ("Ticket & Ewallet", parse_ticket_ewallet),
        ("Game", parse_game),
        ("Streaming & TV", parse_streaming_tv),
        ("Voucher Deals", parse_voucher_deals),
        ("Donation", parse_donation),
        ("Edukasi", parse_edukasi),
        ("Voucher Edukasi", parse_voucher_edukasi),
    ]

    for sheet_name, parser_fn in sheets_to_parse:
        if sheet_name in wb.sheetnames:
            print(f"Parsing {sheet_name}...")
            results = parser_fn(wb[sheet_name])
            all_products.extend(results)
            print(f"  -> {len(results)} products")
        else:
            print(f"SKIP: Sheet '{sheet_name}' not found")

    wb.close()

    # Print summary
    categories = {}
    brands_set = set()
    types = {"prepaid": 0, "postpaid": 0}
    for p in all_products:
        cat = p["category"]
        categories[cat] = categories.get(cat, 0) + 1
        types[p["type"]] += 1
        brands_set.add(p["brand"])

    print(f"\nTotal products: {len(all_products)}")
    print("By category:")
    for cat, count in sorted(categories.items()):
        print(f"  {cat}: {count}")
    print(f"By type:")
    for t, count in types.items():
        print(f"  {t}: {count}")
    print(f"Unique brands: {len(brands_set)}")

    # Deduplicate count
    seen = set()
    for p in all_products:
        seen.add(p["alterra_product_id"])
    print(f"Unique products (by alterra_product_id): {len(seen)}")

    # Generate SQL
    up_sql = generate_sql(all_products)
    down_sql = generate_down_sql()

    with open(UP_PATH, "w", encoding="utf-8") as f:
        f.write(up_sql)
    print(f"\nWritten: {UP_PATH}")

    with open(DOWN_PATH, "w", encoding="utf-8") as f:
        f.write(down_sql)
    print(f"Written: {DOWN_PATH}")


if __name__ == "__main__":
    main()
