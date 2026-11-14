"""Post-process the Rich627-rendered deck: QR code on the closing slide, slide transitions
(diagonal Uncover after each section divider, scroll-up Push elsewhere) and staggered
float-up entrance animations on every slide's content. Usage: python animate.py in.pptx out.pptx"""
import io, re, sys, zipfile
import qrcode
from pptx import Presentation
from pptx.util import Inches, Pt
from pptx.dml.color import RGBColor
from pptx.enum.shapes import MSO_SHAPE

EMU = 914400
SRC, OUT = sys.argv[1], sys.argv[2]
URL = "https://llm-reliability-console.vercel.app"

# ---------- 1. QR code on the closing slide (python-pptx) ----------
prs = Presentation(SRC)
# enlarge the cover's live-console screenshot into a hero image on the right
cover_pic = [s for s in prs.slides[0].shapes if s.shape_type == 13][0]
ratio = cover_pic.image.size[0] / cover_pic.image.size[1]
cover_pic.crop_left = cover_pic.crop_right = cover_pic.crop_top = cover_pic.crop_bottom = 0
cover_pic.width = Inches(6.85); cover_pic.height = int(Inches(6.85) / ratio)
cover_pic.left = Inches(6.2); cover_pic.top = int((Inches(7.5) - cover_pic.height) / 2)
last =prs.slides[len(prs.slides) - 1]
plate = last.shapes.add_shape(MSO_SHAPE.ROUNDED_RECTANGLE, Inches(9.35), Inches(1.55), Inches(3.3), Inches(3.85))
plate.adjustments[0] = 0.06
plate.fill.solid(); plate.fill.fore_color.rgb = RGBColor(0xFF, 0xFF, 0xFF); plate.line.fill.background()
plate.name = "QR plate"
qr = qrcode.QRCode(border=1, box_size=20, error_correction=qrcode.constants.ERROR_CORRECT_M)
qr.add_data(URL); qr.make(fit=True)
buf = io.BytesIO(); qr.make_image(fill_color="#191919", back_color="white").save(buf, "PNG"); buf.seek(0)
pic = last.shapes.add_picture(buf, Inches(9.6), Inches(1.8), Inches(2.8), Inches(2.8)); pic.name = "QR code"
cap = last.shapes.add_textbox(Inches(9.35), Inches(4.7), Inches(3.3), Inches(0.5)); cap.name = "QR caption"
tf = cap.text_frame; tf.margin_left = tf.margin_right = 0
p = tf.paragraphs[0]; p.alignment = 2  # center
r = p.add_run(); r.text = "Scan to open the live console"
r.font.size = Pt(14); r.font.bold = True; r.font.color.rgb = RGBColor(0x7C, 0x5C, 0xFF)
tmp = io.BytesIO(); prs.save(tmp); tmp.seek(0)

# ---------- 2. transitions + animations (raw XML) ----------
P14 = 'xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main"'
MC = 'xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"'

def transition(effect, dur):
    return (f'<mc:AlternateContent {MC}><mc:Choice {P14} Requires="p14">'
            f'<p:transition spd="slow" p14:dur="{dur}">{effect}</p:transition></mc:Choice>'
            f'<mc:Fallback><p:transition spd="slow">{effect}</p:transition></mc:Fallback></mc:AlternateContent>')

def boxes(xml):
    """Top-level shapes in spTree: (kind, id, x, y, w, h, has_text, is_chart) in inches."""
    tree = xml[xml.index("<p:spTree>"):xml.rindex("</p:spTree>")]
    out, depth, i = [], 0, 0
    for m in re.finditer(r"<(/?)p:(sp|pic|graphicFrame|cxnSp|grpSp)\b([^>]*?)(/?)>", tree):
        closing, kind, selfclose = m.group(1), m.group(2), m.group(4)
        if closing:
            depth -= 1
            continue
        if depth == 0:
            end = tree.index(f"</p:{kind}>", m.end()) if kind != "grpSp" else None
            if kind == "grpSp":  # find matching close for groups
                lvl, pos = 1, m.end()
                while lvl:
                    n = re.search(r"<(/?)p:grpSp\b", tree[pos:]); pos += n.end()
                    lvl += -1 if n.group(1) else 1
                end = pos
            body = tree[m.start():end]
            sid = re.search(r'<p:cNvPr id="(\d+)"', body)
            off = re.search(r'<a:off x="(-?\d+)" y="(-?\d+)"\s*/>\s*<a:ext cx="(\d+)" cy="(\d+)"', body)
            if sid and off:
                x, y, w, h = (int(v) / EMU for v in off.groups())
                out.append((kind, sid.group(1), x, y, w, h, "<p:txBody>" in body and kind == "sp", "/chart" in body))
        if not selfclose:
            depth += 1
    return out

def clusters(items):
    """Group shapes whose centre sits inside a bigger 'anchor' shape (card + its text)."""
    groups = []
    for it in sorted(items, key=lambda b: -(b[4] * b[5])):
        cx, cy = it[2] + it[4] / 2, it[3] + it[5] / 2
        for g in groups:
            a = g[0]
            if a[4] * a[5] > it[4] * it[5] * 1.5 and a[2] <= cx <= a[2] + a[4] and a[3] <= cy <= a[3] + a[5]:
                g.append(it); break
        else:
            groups.append([it])
    return sorted(groups, key=lambda g: (round(g[0][3] / 0.6), g[0][2]))

def timing(groups, step=230, dur=650):
    ctn = [3]
    def nid():
        ctn[0] += 1; return ctn[0]
    effects, bld = [], []
    for gi, g in enumerate(groups):
        for it in g:
            spid = it[1]
            a, b, c, d = nid(), nid(), nid(), nid()
            tgt = f'<p:tgtEl><p:spTgt spid="{spid}"/></p:tgtEl>'
            effects.append(
                f'<p:par><p:cTn id="{a}" presetID="42" presetClass="entr" presetSubtype="0" fill="hold" grpId="0" nodeType="withEffect">'
                f'<p:stCondLst><p:cond delay="{gi * step}"/></p:stCondLst><p:childTnLst>'
                f'<p:set><p:cBhvr><p:cTn id="{b}" dur="1" fill="hold"><p:stCondLst><p:cond delay="0"/></p:stCondLst></p:cTn>{tgt}'
                f'<p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set>'
                f'<p:animEffect transition="in" filter="fade"><p:cBhvr><p:cTn id="{c}" dur="{dur}"/>{tgt}</p:cBhvr></p:animEffect>'
                f'<p:anim calcmode="lin" valueType="num"><p:cBhvr><p:cTn id="{d}" dur="{dur}" decel="100000" fill="hold"/>{tgt}'
                f'<p:attrNameLst><p:attrName>ppt_y</p:attrName></p:attrNameLst></p:cBhvr><p:tavLst>'
                f'<p:tav tm="0"><p:val><p:strVal val="#ppt_y+0.06"/></p:val></p:tav>'
                f'<p:tav tm="100000"><p:val><p:strVal val="#ppt_y"/></p:val></p:tav></p:tavLst></p:anim>'
                f'</p:childTnLst></p:cTn></p:par>')
            if it[6]:
                bld.append(f'<p:bldP spid="{spid}" grpId="0" animBg="1"/>')
            elif it[7]:
                bld.append(f'<p:bldGraphic spid="{spid}" grpId="0"><p:bldAsOne/></p:bldGraphic>')
    if not effects:
        return ""
    return ('<p:timing><p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>'
            '<p:seq concurrent="1" nextAc="seek"><p:cTn id="2" dur="indefinite" nodeType="mainSeq"><p:childTnLst>'
            '<p:par><p:cTn id="3" fill="hold"><p:stCondLst><p:cond delay="indefinite"/><p:cond evt="onBegin" delay="0"><p:tn val="2"/></p:cond></p:stCondLst><p:childTnLst>'
            f'<p:par><p:cTn id="{nid()}" fill="hold"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst>'
            + "".join(effects) +
            '</p:childTnLst></p:cTn></p:par></p:childTnLst></p:cTn></p:par>'
            '</p:childTnLst></p:cTn><p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst>'
            '<p:nextCondLst><p:cond evt="onNext" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:nextCondLst></p:seq>'
            '</p:childTnLst></p:cTn></p:par></p:tnLst>'
            + (f'<p:bldLst>{"".join(bld)}</p:bldLst>' if bld else "") + '</p:timing>')

zin = zipfile.ZipFile(tmp)
names = zin.namelist()
order = re.findall(r'r:id="(rId\d+)"', zin.read("ppt/presentation.xml").decode().split("<p:sldIdLst>")[1].split("</p:sldIdLst>")[0])
rels = zin.read("ppt/_rels/presentation.xml.rels").decode()
slide_files = ["ppt/" + re.search(rf'Id="{rid}"[^>]*Target="([^"]+)"|Target="([^"]+)"[^>]*Id="{rid}"', rels).group(1 if re.search(rf'Id="{rid}"[^>]*Target', rels) else 2) for rid in order]
types = sys.argv[3].split(",")  # slide types from deck.json, in order
DARK = {"cover", "section", "next", "closing"}

new = {}
for idx, f in enumerate(slide_files):
    xml = zin.read(f).decode("utf-8")
    t = types[idx]
    prev = types[idx - 1] if idx else None
    if idx == 0:
        tr = transition('<p:fade/>', 1000)
    elif prev == "section":
        tr = transition('<p:pull dir="lu"/>', 1400)   # divider slides away diagonally up-left, revealing the content
    else:
        tr = transition('<p:push dir="u"/>', 900)     # scroll-up feel
    items = []
    for b in boxes(xml):
        kind, sid, x, y, w, h = b[:6]
        if w > 12.5 and h > 6.5:            # full-slide background
            continue
        if y < 0.7 or y >= 6.95:             # kicker chip, section label, footer, page number
            continue
        if t not in DARK and y < 1.85:       # action title + subtitle stay put on light slides
            continue
        items.append(b)
    tm = timing(clusters(items))
    xml = re.sub(r"<p:transition.*?</p:transition>|<p:timing>.*?</p:timing>", "", xml, flags=re.S)
    anchor = "</p:clrMapOvr>" if "</p:clrMapOvr>" in xml else "</p:cSld>"
    xml = xml.replace(anchor, anchor + tr + tm, 1)
    new[f] = xml
    print(f"slide {idx + 1:2d} {t:9s} transition={'uncover-diag' if prev == 'section' else 'push-up' if idx else 'fade'} animated={len(items)}")

with zipfile.ZipFile(OUT, "w", zipfile.ZIP_DEFLATED) as zout:
    for n in names:
        zout.writestr(zin.getinfo(n), new[n].encode("utf-8") if n in new else zin.read(n))
print("wrote", OUT)
