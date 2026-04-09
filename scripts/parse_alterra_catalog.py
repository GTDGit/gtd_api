"""
Parse Alterra product catalog Excel and generate SQL migration.

Usage:
  python scripts/parse_alterra_catalog.py

Reads: docs/ppob/alterra/20262801 - BPA Product Catalogue  (OPEN).xlsx
Writes: migrations/000024_seed_alterra_products.up.sql
        migrations/000024_seed_alterra_products.down.sql
"""

import openpyxl
import os
import re

EXCEL_PATH = os.path.join(os.path.dirname(__file__), "..", "docs", "ppob", "alterra",
                          "20262801 - BPA Product Catalogue  (OPEN).xlsx")
UP_PATH = os.path.join(os.path.dirname(__file__), "..", "migrations", "000024_seed_alterra_products.up.sql")
DOWN_PATH = os.path.join(os.path.dirname(__file__), "..", "migrations", "000024_seed_alterra_products.down.sql")

ALTERRA_PROVIDER_ID = 2  # From ppob_providers table


def escape_sql(s):
    if s is None:
        return ""
    return str(s).replace("'", "''").strip()


def parse_pulsa(ws):
    """Parse Pulsa sheet - prepaid mobile top-up."""
    products = []
    for i, row in enumerate(ws.iter_rows(min_row=2, values_only=True)):
        product_id, tipe, name, denom, operator, price_type, sell_price = row[:7]
        if product_id is None or not isinstance(product_id, (int, float)):
            continue
        product_id = int(product_id)
        name = str(name).strip() if name else ""
        operator = str(operator).strip() if operator else ""
        sell_price = int(sell_price) if sell_price and isinstance(sell_price, (int, float)) else 0
        denom = int(denom) if denom and isinstance(denom, (int, float)) else 0

        brand = operator.upper().replace(" ", "")
        if "axis" in operator.lower():
            brand = "AXIS"
        elif "xl" in operator.lower():
            brand = "XL"
        elif "telkomsel" in operator.lower():
            brand = "TELKOMSEL"
        elif "indosat" in operator.lower() or "im3" in operator.lower():
            brand = "INDOSAT"
        elif "tri" in operator.lower() or "three" in operator.lower():
            brand = "TRI"
        elif "smartfren" in operator.lower():
            brand = "SMARTFREN"
        elif "by.u" in operator.lower():
            brand = "BYU"

        sku_code = f"ALT-PULSA-{brand}-{denom}"
        products.append({
            "sku_code": sku_code,
            "name": name,
            "category": "Pulsa",
            "brand": brand,
            "type": "prepaid",
            "admin": 0,
            "commission": 0,
            "description": f"Alterra {name}",
            "alterra_product_id": product_id,
            "price": sell_price,
        })
    return products


def parse_pln_bpjs_postpaid(ws):
    """Parse 'PLN, BPJS & Postpaid' sheet - has multiple sections."""
    products = []
    current_section = None

    for i, row in enumerate(ws.iter_rows(values_only=True)):
        vals = list(row)[:7]
        first_val = vals[0]

        # Detect section headers
        if isinstance(first_val, str):
            lower = first_val.lower()
            if "pln prepaid" in lower:
                current_section = "pln_prepaid"
                continue
            elif "pln postpaid" in lower:
                current_section = "pln_postpaid"
                continue
            elif "bpjs" in lower and "product" in lower:
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
            elif first_val == "Product ID" or first_val == "Product id":
                continue  # Skip header rows

        if not isinstance(first_val, (int, float)) or first_val is None:
            continue

        product_id = int(first_val)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""
        operator = str(vals[3]).strip() if vals[3] else ""
        service_fee = vals[4]
        if isinstance(service_fee, (int, float)):
            price = int(service_fee)
        else:
            price = 0

        # Skip insurance products (not in our category list)
        if current_section == "insurance":
            continue

        # Determine category, brand, type based on section
        if current_section == "pln_prepaid":
            category = "PLN"
            brand = "PLN"
            prod_type = "prepaid"
            denom = 0
            # Extract denomination from name
            m = re.search(r'(\d[\d,.]+)', name.replace("Rp. ", "").replace(",", ""))
            if m:
                denom = int(m.group(1))
            sku_code = f"ALT-PLN-TOKEN-{denom}"
        elif current_section == "pln_postpaid":
            category = "PLN"
            brand = "PLN"
            prod_type = "postpaid"
            sku_code = f"ALT-PLN-POSTPAID"
        elif current_section == "bpjs":
            category = "BPJS"
            brand = operator.upper().replace(" ", "-")
            prod_type = "postpaid"
            sku_code = f"ALT-BPJS-{product_id}"
        elif current_section == "telkom_postpaid":
            category = "Pascabayar"
            brand = "TELKOM"
            prod_type = "postpaid"
            sku_code = f"ALT-TELKOM-POSTPAID"
        elif current_section == "mobile_postpaid":
            category = "Pascabayar"
            brand = operator.upper().replace(" ", "")
            if "telkomsel" in operator.lower():
                brand = "TELKOMSEL"
            elif "xl" in operator.lower():
                brand = "XL"
            elif "indosat" in operator.lower():
                brand = "INDOSAT"
            elif "tri" in operator.lower():
                brand = "TRI"
            elif "smartfren" in operator.lower():
                brand = "SMARTFREN"
            prod_type = "postpaid"
            sku_code = f"ALT-POSTPAID-{brand}"
        elif current_section == "internet_service":
            category = "Pascabayar"
            brand = operator.upper().replace(" ", "-")
            prod_type = "postpaid"
            sku_code = f"ALT-INET-{product_id}"
        else:
            continue

        products.append({
            "sku_code": sku_code,
            "name": name,
            "category": category,
            "brand": brand,
            "type": prod_type,
            "admin": 0,
            "commission": 0,
            "description": f"Alterra {name}",
            "alterra_product_id": product_id,
            "price": price,
        })
    return products


def parse_gas_pdam(ws):
    """Parse Gas & PDAM sheet - postpaid utilities."""
    products = []
    current_section = None

    for i, row in enumerate(ws.iter_rows(values_only=True)):
        vals = list(row)[:7]
        first_val = vals[0]

        if isinstance(first_val, str):
            lower = first_val.lower()
            if lower == "gas":
                current_section = "gas"
                continue
            elif lower == "pdam":
                current_section = "pdam"
                continue
            elif first_val in ("Product ID", "Product id"):
                continue

        if not isinstance(first_val, (int, float)) or first_val is None:
            continue

        product_id = int(first_val)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""

        if current_section == "gas":
            operator = str(vals[4]).strip() if vals[4] else name
            price = int(vals[5]) if vals[5] and isinstance(vals[5], (int, float)) else 0
            brand = operator.upper().replace(" ", "-")
            category = "Gas & PDAM"
            sku_code = f"ALT-GAS-{product_id}"
        elif current_section == "pdam":
            operator = str(vals[4]).strip() if vals[4] else ""
            price = int(vals[5]) if vals[5] and isinstance(vals[5], (int, float)) else 0
            # Check status column
            status = str(vals[6]).strip().upper() if vals[6] else "OPEN"
            if status != "OPEN":
                continue
            brand = "PDAM"
            category = "Gas & PDAM"
            sku_code = f"ALT-PDAM-{product_id}"
        else:
            continue

        products.append({
            "sku_code": sku_code,
            "name": name,
            "category": category,
            "brand": brand,
            "type": "postpaid",
            "admin": 0,
            "commission": 0,
            "description": f"Alterra {name}",
            "alterra_product_id": product_id,
            "price": price,
        })
    return products


def parse_streaming_tv(ws):
    """Parse Streaming & TV sheet - prepaid vouchers and postpaid subscriptions."""
    products = []
    current_section = None

    for i, row in enumerate(ws.iter_rows(values_only=True)):
        vals = list(row)[:7]
        first_val = vals[0]

        if isinstance(first_val, str):
            lower = first_val.lower()
            if "streaming voucher" in lower:
                current_section = "streaming_voucher"
                continue
            elif "tv" in lower and ("postpaid" in lower or "cable" in lower or "tagihan" in lower):
                current_section = "tv_postpaid"
                continue
            elif first_val in ("Product ID", "Product id"):
                continue

        if not isinstance(first_val, (int, float)) or first_val is None:
            continue

        product_id = int(first_val)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""
        denom = vals[3]
        publisher = str(vals[4]).strip() if vals[4] else ""
        price = int(vals[5]) if vals[5] and isinstance(vals[5], (int, float)) else 0

        # Determine if prepaid voucher or postpaid subscription
        tipe_lower = tipe.lower()
        if "voucher" in tipe_lower or "topup" in tipe_lower:
            prod_type = "prepaid"
        elif "postpaid" in tipe_lower or "tagihan" in tipe_lower:
            prod_type = "postpaid"
        else:
            prod_type = "prepaid"  # Default streaming vouchers to prepaid

        brand = publisher.upper().replace(" ", "-")
        products.append({
            "sku_code": f"ALT-STREAM-{product_id}",
            "name": name,
            "category": "Streaming & TV",
            "brand": brand,
            "type": prod_type,
            "admin": 0,
            "commission": 0,
            "description": f"Alterra {name}",
            "alterra_product_id": product_id,
            "price": price,
        })
    return products


def parse_voucher_deals(ws):
    """Parse Voucher Deals sheet - prepaid vouchers."""
    products = []
    for i, row in enumerate(ws.iter_rows(min_row=2, values_only=True)):
        vals = list(row)[:6]
        product_id = vals[0]
        if not isinstance(product_id, (int, float)) or product_id is None:
            continue

        product_id = int(product_id)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""
        denom = int(vals[3]) if vals[3] and isinstance(vals[3], (int, float)) else 0
        operator = str(vals[4]).strip() if vals[4] else ""
        price = int(vals[5]) if vals[5] and isinstance(vals[5], (int, float)) else 0

        brand = operator.upper().replace(" ", "-")
        products.append({
            "sku_code": f"ALT-VOUCHER-{product_id}",
            "name": name,
            "category": "Voucher Deals",
            "brand": brand,
            "type": "prepaid",
            "admin": 0,
            "commission": 0,
            "description": f"Alterra {name}",
            "alterra_product_id": product_id,
            "price": price,
        })
    return products


def parse_edukasi(ws):
    """Parse Edukasi sheet - postpaid education payments."""
    products = []
    for i, row in enumerate(ws.iter_rows(min_row=2, values_only=True)):
        vals = list(row)[:7]
        product_id = vals[0]
        if not isinstance(product_id, (int, float)) or product_id is None:
            continue

        product_id = int(product_id)
        tipe = str(vals[1]).strip() if vals[1] else ""
        name = str(vals[2]).strip() if vals[2] else ""
        operator = str(vals[3]).strip() if vals[3] else ""
        price = int(vals[4]) if vals[4] and isinstance(vals[4], (int, float)) else 0

        brand = operator.upper().replace(" ", "-")
        products.append({
            "sku_code": f"ALT-EDU-{product_id}",
            "name": name,
            "category": "Edukasi",
            "brand": brand,
            "type": "postpaid",
            "admin": 0,
            "commission": 0,
            "description": f"Alterra {name}",
            "alterra_product_id": product_id,
            "price": price,
        })
    return products


def generate_sql(all_products):
    """Generate SQL migration from parsed products."""
    # Deduplicate by alterra_product_id
    seen = set()
    unique = []
    for p in all_products:
        if p["alterra_product_id"] not in seen:
            seen.add(p["alterra_product_id"])
            unique.append(p)

    # Also deduplicate sku_codes by appending product_id if collision
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

    lines = []
    lines.append("-- Alterra product catalog seeding")
    lines.append("-- Auto-generated by scripts/parse_alterra_catalog.py")
    lines.append("-- Categories: Pulsa, PLN, BPJS, Pascabayar, Gas & PDAM, Streaming & TV, Voucher Deals, Edukasi")
    lines.append("")
    lines.append("BEGIN;")
    lines.append("")

    # Insert products with ON CONFLICT DO NOTHING
    lines.append("-- Insert products (skip if sku_code already exists)")
    for p in unique:
        lines.append(
            f"INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active) "
            f"VALUES ('{escape_sql(p['sku_code'])}', '{escape_sql(p['name'])}', '{escape_sql(p['category'])}', "
            f"'{escape_sql(p['brand'])}', '{p['type']}', {p['admin']}, {p['commission']}, "
            f"'{escape_sql(p['description'])}', true) "
            f"ON CONFLICT (sku_code) DO NOTHING;"
        )

    lines.append("")
    lines.append("-- Insert provider SKU mappings for Alterra (provider_id = 2)")
    lines.append("-- provider_sku_code = Alterra product_id as string")

    for p in unique:
        lines.append(
            f"INSERT INTO ppob_provider_skus (provider_id, product_id, provider_sku_code, provider_product_name, price, admin, commission, is_active, is_available) "
            f"SELECT {ALTERRA_PROVIDER_ID}, p.id, '{p['alterra_product_id']}', '{escape_sql(p['name'])}', "
            f"{p['price']}, {p['admin']}, {p['commission']}, true, true "
            f"FROM products p WHERE p.sku_code = '{escape_sql(p['sku_code'])}' "
            f"ON CONFLICT (provider_id, provider_sku_code) DO UPDATE SET "
            f"price = EXCLUDED.price, provider_product_name = EXCLUDED.provider_product_name, "
            f"is_active = true, is_available = true;"
        )

    lines.append("")
    lines.append("COMMIT;")
    return "\n".join(lines)


def generate_down_sql():
    return (
        "-- Remove Alterra provider SKU mappings\n"
        "DELETE FROM ppob_provider_skus WHERE provider_id = 2;\n"
        "\n"
        "-- Remove Alterra-only products (products that have no other provider SKUs)\n"
        "DELETE FROM products WHERE sku_code LIKE 'ALT-%' AND id NOT IN (\n"
        "    SELECT DISTINCT product_id FROM ppob_provider_skus\n"
        ");\n"
    )


def main():
    wb = openpyxl.load_workbook(EXCEL_PATH, read_only=True)

    all_products = []

    # Parse each target sheet
    print("Parsing Pulsa...")
    all_products.extend(parse_pulsa(wb["Pulsa"]))

    print("Parsing PLN, BPJS & Postpaid...")
    all_products.extend(parse_pln_bpjs_postpaid(wb["PLN, BPJS & Postpaid"]))

    print("Parsing Gas & PDAM...")
    all_products.extend(parse_gas_pdam(wb["Gas & PDAM"]))

    print("Parsing Streaming & TV...")
    all_products.extend(parse_streaming_tv(wb["Streaming & TV"]))

    print("Parsing Voucher Deals...")
    all_products.extend(parse_voucher_deals(wb["Voucher Deals"]))

    print("Parsing Edukasi...")
    all_products.extend(parse_edukasi(wb["Edukasi"]))

    wb.close()

    # Print summary
    categories = {}
    types = {"prepaid": 0, "postpaid": 0}
    for p in all_products:
        cat = p["category"]
        categories[cat] = categories.get(cat, 0) + 1
        types[p["type"]] += 1

    print(f"\nTotal products: {len(all_products)}")
    print("By category:")
    for cat, count in sorted(categories.items()):
        print(f"  {cat}: {count}")
    print(f"By type:")
    for t, count in types.items():
        print(f"  {t}: {count}")

    # Deduplicate
    seen = set()
    unique = 0
    for p in all_products:
        if p["alterra_product_id"] not in seen:
            seen.add(p["alterra_product_id"])
            unique += 1
    print(f"Unique products (by alterra_product_id): {unique}")

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
