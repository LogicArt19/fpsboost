from PIL import Image, ImageDraw
S=1024
img=Image.new("RGBA",(S,S),(0,0,0,0))
# fond dégradé violet -> cyan
grad=Image.new("RGBA",(S,S))
gp=grad.load()
for y in range(S):
    for x in range(S):
        t=(x+y)/(2*S)
        gp[x,y]=(int(0x8B+(0x22-0x8B)*t),int(0x5C+(0xD3-0x5C)*t),int(0xF6+(0xEE-0xF6)*t),255)
mask=Image.new("L",(S,S),0)
ImageDraw.Draw(mask).rounded_rectangle([40,40,S-40,S-40],radius=220,fill=255)
img.paste(grad,(0,0),mask)
d=ImageDraw.Draw(img)
d.rounded_rectangle([90,90,S-90,S-90],radius=180,fill=(13,15,21,255))
# éclair dégradé
bolt=[(590,150),(300,560),(490,560),(420,880),(730,440),(535,440),(620,150)]
bm=Image.new("L",(S,S),0); ImageDraw.Draw(bm).polygon(bolt,fill=255)
img.paste(grad,(0,0),bm)
img=img.resize((256,256),Image.LANCZOS)
img.save("icon.ico",sizes=[(16,16),(20,20),(24,24),(32,32),(40,40),(48,48),(64,64),(128,128),(256,256)])
img.save("icon_preview.png")
