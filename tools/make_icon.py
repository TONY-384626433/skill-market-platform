# -*- coding: utf-8 -*-
"""生成 SkillHub 桌面快捷方式图标 (多尺寸 .ico)"""
import os
from PIL import Image, ImageDraw

OUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "assets")
os.makedirs(OUT_DIR, exist_ok=True)
OUT = os.path.join(OUT_DIR, "skillhub.ico")

NAVY = (21, 36, 52, 255)
BLUE = (11, 99, 206, 255)
TEAL = (112, 212, 193, 255)
WHITE = (255, 255, 255, 255)


def rounded(draw, box, radius, fill):
    draw.rounded_rectangle(box, radius=radius, fill=fill)


def build(size):
    scale = size / 256.0
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    def s(v):
        return v * scale

    # 外框: 深色圆角方块
    rounded(d, (s(6), s(6), s(250), s(250)), s(54), NAVY)
    # 内层: 品牌蓝圆角方块
    rounded(d, (s(38), s(38), s(218), s(218)), s(34), BLUE)
    # 银行立柱 / 技能柱状 (4 根白色柱, 高低错落)
    bars = [(62, 150, 92, 190), (104, 118, 134, 190), (146, 92, 176, 190), (188, 134, 214, 190)]
    for x0, y0, x1, y1 in bars:
        rounded(d, (s(x0), s(y0), s(x1), s(y1)), s(9), WHITE)
    # 顶部青色装饰线
    rounded(d, (s(62), s(60), s(214), s(74)), s(7), TEAL)
    return img


sizes = [256, 128, 64, 48, 32, 16]
frames = [build(n) for n in sizes]
frames[0].save(OUT, format="ICO", sizes=[(n, n) for n in sizes])
print("icon written:", OUT, os.path.getsize(OUT), "bytes")
