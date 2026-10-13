"""Extract the supplied Swappie category snapshot and localize its card images."""

import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urljoin

import requests
from bs4 import BeautifulSoup


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "imports" / "swappie-iphone-catalog.json"
IMAGE_DIR = ROOT / "medusa-store" / "apps" / "storefront" / "public" / "swappie"
SOURCE_URL = "https://swappie.com/en/iphone/"
RELEASE_YEAR_REFERENCE = "https://support.apple.com/en-us/108044"
# Swappie's releaseYear values are internal IDs, not calendar years. The
# mapping below was checked against Apple's model identification page.
RELEASE_YEAR_BY_CODE = {
    17: 2019,
    18: 2019,
    20: 2020,
    21: 2020,
    22: 2020,
    23: 2020,
    24: 2020,
    25: 2021,
    26: 2021,
    27: 2021,
    28: 2022,
    29: 2022,
    30: 2022,
    31: 2022,
    32: 2022,
    33: 2023,
    34: 2023,
    35: 2023,
    36: 2023,
    37: 2024,
    38: 2024,
    39: 2024,
    40: 2024,
    41: 2025,
    42: 2025,
    43: 2026,
}

# The category card renders colour dots in a different order from the
# availableColors array. Three older cards also render more dots than that
# array contains. Names below match the rendered dots, in DOM order.
CARD_COLOR_NAMES = {
    "iPhone 14": "Midnight|Purple|Starlight|Red|Blue|Yellow",
    "iPhone 15 Pro": "Black Titanium|White Titanium|Blue Titanium|Natural Titanium",
    "iPhone 15": "Pink|Green|Black|Blue|Yellow",
    "iPhone 13": "Pink|Blue|Midnight|Starlight|Red|Green",
    "iPhone 14 Pro": "Space Black|Silver|Gold|Deep Purple",
    "iPhone 15 Pro Max": "Black Titanium|White Titanium|Blue Titanium|Natural Titanium",
    "iPhone 13 Pro": "Sierra Blue|Silver|Gold|Graphite|Alpine Green",
    "iPhone 13 mini": "Pink|Blue|Midnight|Starlight|Red|Green",
    "iPhone 12": "Black|White|Red|Green|Blue|Purple",
    "iPhone 16 Pro": "Black Titanium|White Titanium|Natural Titanium|Desert Titanium",
    "iPhone 14 Pro Max": "Space Black|Silver|Gold|Deep Purple",
    "iPhone 16 Pro Max": "Black Titanium|White Titanium|Natural Titanium|Desert Titanium",
    "iPhone 12 mini": "Black|White|Red|Green|Blue|Purple",
    "iPhone 13 Pro Max": "Sierra Blue|Silver|Gold|Graphite|Alpine Green",
    "iPhone 11": "Black|White|Red|Purple|Yellow|Green",
    "iPhone 15 Plus": "Pink|Green|Black|Blue|Yellow",
    "iPhone 16": "Ultramarine|Teal|Pink|White|Black",
    "iPhone 12 Pro": "Silver|Gold|Pacific Blue|Graphite",
    "iPhone SE 2022": "Midnight|Starlight|Red",
    "iPhone 14 Plus": "Midnight|Purple|Starlight|Red|Blue|Yellow",
    "iPhone 12 Pro Max": "Silver|Gold|Pacific Blue|Graphite",
    "iPhone 11 Pro": "Space Gray|Silver|Gold|Midnight Green",
    "iPhone 16 Plus": "Ultramarine|Teal|Pink|White|Black",
    "iPhone 17 Pro": "Silver|Cosmic Orange|Deep Blue",
    "iPhone 16e": "White|Black",
    "iPhone 17 Pro Max": "Silver|Cosmic Orange|Deep Blue",
    "iPhone SE 2020": "Black|White|Red",
    "iPhone Air": "Space Black|Cloud White|Light Gold|Sky Blue",
    "iPhone 17": "Black|White|Mist Blue|Sage|Lavender",
    "iPhone 17e": "Black|White|Soft Pink",
}


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("Usage: python extract-swappie-html.py <saved-html-path>")

    source = Path(sys.argv[1])
    html = source.read_text(encoding="utf-8")
    soup = BeautifulSoup(html, "html.parser")
    script = next(
        tag.string
        for tag in soup.find_all("script")
        if tag.string and "window.__INITIAL__DATA__" in tag.string
    )
    payload = script.split("window.__INITIAL__DATA__ = ", 1)[1].lstrip()
    data = json.JSONDecoder().raw_decode(payload)[0]
    items = data["categoryPageData"]["items"]

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    IMAGE_DIR.mkdir(parents=True, exist_ok=True)
    products = []
    session = requests.Session()
    session.headers["User-Agent"] = "Mozilla/5.0 (compatible; local-catalog-import/1.0)"

    for item in items:
        model = item["name"].strip()
        sku = item["sku"].strip()
        slug = item["url"].strip("/").split("/")[-1]
        anchor = soup.find("a", href=item["url"])
        image = anchor.find("img") if anchor else None
        if not image or not image.get("src"):
            raise ValueError(f"Missing image URL for {model}")
        dots = anchor.select('div[class*="ModelCard__ColorDot"]')
        card_color_names = CARD_COLOR_NAMES[model].split("|")
        if len(dots) != len(card_color_names):
            raise ValueError(f"Card colour count mismatch for {model}")
        card_colors = []
        for name, dot in zip(card_color_names, dots):
            hex_value = dot.get("color", "")
            if not re.fullmatch(r"#[0-9a-fA-F]{6}", hex_value):
                raise ValueError(f"Invalid card colour for {model}: {hex_value}")
            card_colors.append({"name": name, "hex": hex_value})

        original_image_url = urljoin(SOURCE_URL, image["src"])
        high_resolution_url = re.sub(
            r"width=240,height=240", "width=600,height=600", original_image_url
        )
        filename = Path(item["img"]).name
        target = IMAGE_DIR / filename
        if target.exists():
            image_bytes = target.read_bytes()
        else:
            response = session.get(high_resolution_url, timeout=30)
            response.raise_for_status()
            if not response.headers.get("content-type", "").startswith("image/"):
                raise ValueError(f"Unexpected image response for {model}")
            image_bytes = response.content
            target.write_bytes(image_bytes)

        price_cents = int(item["normalPrice"])
        release_year_code = int(item["releaseYear"])
        if release_year_code not in RELEASE_YEAR_BY_CODE:
            raise ValueError(f"Unknown releaseYear code {release_year_code} for {model}")
        products.append(
            {
                "model": model,
                "slug": slug,
                "sku": sku,
                "selected_storage_gb": int(item["storage"]),
                "available_storages_gb": [int(n) for n in item["availableStorages"]],
                "available_colors": item["availableColors"],
                "card_colors": card_colors,
                "price_eur": price_cents / 100,
                "currency": "EUR",
                "source_release_year_code": release_year_code,
                "release_year": RELEASE_YEAR_BY_CODE[release_year_code],
                "source_url": urljoin(SOURCE_URL, item["url"]),
                "source_image_url": original_image_url,
                "local_image_url": f"http://127.0.0.1:8000/swappie/{filename}",
                "image_sha256": hashlib.sha256(image_bytes).hexdigest(),
            }
        )

    skus = [product["sku"] for product in products]
    if len(skus) != len(set(skus)):
        raise ValueError("Duplicate SKUs in source HTML")

    manifest = {
        "source_url": SOURCE_URL,
        "source_file_sha256": hashlib.sha256(source.read_bytes()).hexdigest(),
        "extracted_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Category cards: one verified SKU and starting price per model",
        "release_year_reference": RELEASE_YEAR_REFERENCE,
        "products": products,
    }
    OUTPUT.write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"Extracted {len(products)} products and localized {len(products)} images")
    print(f"Manifest: {OUTPUT}")


if __name__ == "__main__":
    main()
