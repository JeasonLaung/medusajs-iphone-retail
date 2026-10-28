"""Compare controlled browser PNGs, allowing one RGB level of compositor rounding.

No shifted pixels, resized image, missing region, or structural tolerance.
The browser test independently compares region text and checks interaction.
"""
import json
import sys
from PIL import Image, ImageChops

with Image.open(sys.argv[1]) as left, Image.open(sys.argv[2]) as right:
    if left.size != right.size:
        raise RuntimeError('Screenshot dimensions differ')
    delta = ImageChops.difference(left.convert('RGB'), right.convert('RGB'))
    maximum = max(high for low, high in delta.getextrema())
    changed = sum(pixel != (0, 0, 0) for pixel in delta.get_flattened_data())
    print(json.dumps({'max_channel_delta': maximum, 'changed_pixels': changed,
                      'total_pixels': left.width * left.height,
                      'max_allowed_channel_delta': 1}))
    if maximum > 1:
        sys.exit(1)
