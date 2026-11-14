from PIL import Image, ImageDraw, ImageFilter
def rounded(im, r):
    m = Image.new('L', im.size, 0); ImageDraw.Draw(m).rounded_rectangle([0,0,im.size[0]-1,im.size[1]-1], r, fill=255)
    out = im.convert('RGBA'); out.putalpha(m); return out
def shadow_paste(canvas, im, xy, blur=40, off=18, alpha=170, glow=None):
    sh = Image.new('RGBA', canvas.size, (0,0,0,0))
    m = Image.new('L', im.size, 0); ImageDraw.Draw(m).rounded_rectangle([0,0,im.size[0]-1,im.size[1]-1], 28, fill=alpha)
    col = Image.new('RGBA', im.size, glow or (0,0,0,255)); col.putalpha(m)
    sh.paste(col, (xy[0], xy[1]+off), col); sh = sh.filter(ImageFilter.GaussianBlur(blur))
    canvas.alpha_composite(sh); canvas.alpha_composite(im, xy)
report = Image.open('shots/report.png').convert('RGB')
run = Image.open('shots/run-02.png').convert('RGB')
W,H = 3500, 2150
cv = Image.new('RGBA', (W,H), (0,0,0,0))
main = rounded(report.resize((2900, 1631), Image.LANCZOS), 30)
ImageDraw.Draw(main).rounded_rectangle([1,1,main.width-2,main.height-2], 30, outline=(86,78,130,255), width=5)
shadow_paste(cv, main, (40, 30))
panel = run.crop((2425, 305, 3160, 880))            # Progress panel, mid-run
panel = panel.resize((int(panel.width*1.15), int(panel.height*1.15)), Image.LANCZOS)
p = rounded(panel, 26)
d = ImageDraw.Draw(p); d.rounded_rectangle([1,1,p.width-2,p.height-2], 26, outline=(124,92,255,255), width=5)
shadow_paste(cv, p, (W - p.width - 40, 1290), blur=55, off=0, alpha=230, glow=(124,92,255,255))
cv.save('console-live.png'); print(cv.size)
