"""Verify display sizes and rear camera counts for imported iPhones."""

import json
import re
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urljoin
from concurrent.futures import ThreadPoolExecutor
from time import sleep

import requests
from bs4 import BeautifulSoup


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "imports" / "swappie-iphone-catalog.json"
OUTPUT = ROOT / "imports" / "iphone-model-specs.json"
APPLE_MODELS_URL = "https://support.apple.com/en-us/108044"
APPLE_SE_2020_URL = "https://support.apple.com/en-us/111882"
ALIASES = {
    "iPhone SE 2020": "iPhone SE (2nd generation)",
    "iPhone SE 2022": "iPhone SE (3rd generation)",
}
CAMERA_WORDS = {"one": 1, "two": 2, "three": 3}


def fetch(url: str) -> requests.Response:
    for attempt in range(5):
        try:
            response = requests.get(url, timeout=40)
            response.raise_for_status()
            return response
        except requests.RequestException:
            if attempt == 4:
                raise
            sleep(2 ** attempt)
    raise RuntimeError(f"Could not fetch {url}")


def normalize(value: str) -> str:
    return " ".join(value.replace("\u2011", "-").replace("\u2013", "-").split())


def section_items(soup: BeautifulSoup, heading_prefix: str) -> list[str]:
    heading = next(
        (h for h in soup.find_all("h3") if normalize(h.get_text(" ", strip=True)).startswith(heading_prefix)),
        None,
    )
    if heading is None:
        return []
    values = []
    for element in heading.next_elements:
        if getattr(element, "name", None) in ("h2", "h3"):
            break
        if getattr(element, "name", None) in ("p", "li"):
            value = normalize(element.get_text(" ", strip=True))
            if value and value not in values:
                values.append(value)
    return values


def metric_value(items: list[str], label: str, unit: str) -> float | None:
    text = " ".join(items)
    match = re.search(rf"{label}\s*:.*?\(([\d.]+) {unit}\)", text, re.IGNORECASE)
    return float(match.group(1)) if match else None


def read_tech_specs(source: dict) -> dict:
    response = fetch(source["tech_specs_url"])
    soup = BeautifulSoup(response.text, "html.parser")
    display = section_items(soup, "Display")
    chip = section_items(soup, "Chip")
    camera = section_items(soup, "Camera")
    front = section_items(soup, "TrueDepth Camera") or section_items(soup, "Front Camera")
    size = section_items(soup, "Size and Weight")
    connectors = " ".join(section_items(soup, "External Buttons and Connectors"))
    headings = {normalize(h.get_text(" ", strip=True)) for h in soup.find_all("h3")}
    display_text = " ".join(display)
    resolution = re.search(r"(\d{3,4})-by-(\d{3,4})-pixel", display_text)
    ppi = re.search(r"\b(\d{3}) ppi\b", display_text)

    source.update(
        {
            "tech_specs_url": response.url,
            "display_name": display[0] if display else None,
            "display_resolution_px":
                f"{resolution.group(1)} x {resolution.group(2)}" if resolution else None,
            "pixel_density_ppi": int(ppi.group(1)) if ppi else None,
            "chip": chip[0] if chip else None,
            "rear_camera_system": camera[0] if camera else None,
            "front_camera": front[0] if front else None,
            "width_mm": metric_value(size, "Width", "mm"),
            "height_mm": metric_value(size, "Height", "mm"),
            "depth_mm": metric_value(size, "Depth", "mm"),
            "weight_g": metric_value(size, "Weight", "grams"),
            "connector": "USB-C" if "USB-C" in connectors else "Lightning" if "Lightning" in connectors else source["connector"],
            "biometric": "Face ID" if "Face ID" in headings else "Touch ID" if "Touch ID" in headings else source["biometric"],
        }
    )
    return source


def main() -> None:
    response = fetch(APPLE_MODELS_URL)
    soup = BeautifulSoup(response.text, "html.parser")
    headings = {h.get_text(" ", strip=True): h for h in soup.find_all("h2")}
    products = json.loads(CATALOG.read_text(encoding="utf-8"))["products"]
    specs = []

    for product in products:
        model = product["model"]
        heading = headings.get(ALIASES.get(model, model))
        if heading is None:
            raise ValueError(f"Apple model not found: {model}")
        paragraphs = []
        tech_specs_url = None
        for element in heading.next_elements:
            if getattr(element, "name", None) == "h2":
                break
            if getattr(element, "name", None) == "p":
                paragraphs.append(element.get_text(" ", strip=True))
            if getattr(element, "name", None) == "a" and "tech specs" in element.get_text(" ", strip=True).lower():
                tech_specs_url = urljoin(APPLE_MODELS_URL, element.get("href", ""))
        details = " ".join(paragraphs)
        if not tech_specs_url:
            raise ValueError(f"Technical specifications link not found: {model}")
        display = re.search(r"\b(\d\.\d)[- ]inch(?:es)?\b", details)
        if not display:
            raise ValueError(f"Display size not found: {model}")

        camera = re.search(
            r"(?:There are|There's) (one|two|three)(?: \d+ MP)? cameras? on the back",
            details,
            re.IGNORECASE,
        )
        if camera:
            camera_count = CAMERA_WORDS[camera.group(1).lower()]
            camera_reference = APPLE_MODELS_URL
        elif model == "iPhone SE 2022" and "There's a 12 MP Wide camera on the back" in details:
            camera_count = 1
            camera_reference = APPLE_MODELS_URL
        elif model == "iPhone SE 2020":
            # Apple's model identification page omits its camera count; the
            # separate Apple technical specifications describe one Wide camera.
            camera_count = 1
            camera_reference = APPLE_SE_2020_URL
        else:
            raise ValueError(f"Rear camera count not found: {model}")

        fields = {}
        for paragraph in paragraphs:
            for field in ("Year introduced", "Capacity", "Colors", "Model numbers"):
                if paragraph.startswith(field + ":"):
                    fields[field] = paragraph.split(":", 1)[1].strip()
        if len(fields) != 4:
            raise ValueError(f"Identification fields missing for {model}: {fields.keys()}")
        model_numbers = list(dict.fromkeys(re.findall(r"\bA\d{4}\b", fields["Model numbers"])))
        if not model_numbers:
            raise ValueError(f"Model numbers missing for {model}")

        specs.append(
            {
                "model": model,
                "release_year": int(fields["Year introduced"]),
                "apple_capacities": fields["Capacity"],
                "apple_colors": [color.strip() for color in fields["Colors"].split(",")],
                "apple_model_numbers": model_numbers,
                "display_size_inches": float(display.group(1)),
                "rear_camera_count": camera_count,
                "connector": "USB-C" if "USB-C connector" in details else "Lightning" if "Lightning connector" in details else None,
                "biometric": "Touch ID" if "Touch ID" in details else "Face ID" if "Face ID" in details else None,
                "display_source_url": APPLE_MODELS_URL,
                "camera_source_url": camera_reference,
                "tech_specs_url": tech_specs_url,
            }
        )

    if len(specs) != 30 or len({item["model"] for item in specs}) != 30:
        raise ValueError("Expected exactly 30 unique model specifications")

    with ThreadPoolExecutor(max_workers=2) as executor:
        specs = list(executor.map(read_tech_specs, specs))

    required = ["display_name", "display_resolution_px", "pixel_density_ppi", "chip", "rear_camera_system", "width_mm", "height_mm", "depth_mm", "weight_g"]
    for spec in specs:
        missing = [field for field in required if spec[field] is None]
        if missing:
            print(f"Missing technical fields for {spec['model']}: {', '.join(missing)}")

    OUTPUT.write_text(
        json.dumps(
            {
                "verified_at": datetime.now(timezone.utc).isoformat(),
                "scope": "Model-level Apple identification and technical specifications; not SKU data",
                "products": specs,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )
    print(f"Verified {len(specs)} iPhone model specifications: {OUTPUT}")


if __name__ == "__main__":
    main()
