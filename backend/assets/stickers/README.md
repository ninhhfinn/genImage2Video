# Assets trang trí — mode Thuyết minh AI

Toàn bộ asset trong thư mục này là **tự sinh bằng tool trên máy** (headless Chrome + ffmpeg + Pillow),
không tải từ web → **không vấn đề bản quyền**.

## meme_cat.png (500×560, PNG RGBA, nền trong suốt)

Sticker mèo mắt tim 😻 + tay trái tim 🫶 kiểu die-cut TikTok (viền sticker trắng ~10px quanh hình).

Cách tạo:
1. Render HTML chứa 2 emoji (font **Noto Color Emoji** hệ thống, cỡ 360px + 150px, xoay nhẹ −6°/+4°)
   bằng headless Chrome, nền trong suốt — cùng kiểu gọi với `chrome_overlay.go`:
   `google-chrome --headless --no-sandbox --disable-gpu --hide-scrollbars --force-device-scale-factor=1 --default-background-color=00000000 --screenshot=... --window-size=500,560 <file.html>`
2. Thêm viền trắng die-cut bằng Python/Pillow: lấy kênh alpha → threshold → dilate ~10px
   (`MaxFilter(21)`) → bo tròn góc (GaussianBlur + re-threshold) → đổ trắng → composite emoji lên trên.

License: emoji từ font Noto Color Emoji (**OFL/Apache 2.0**, cài sẵn trên hệ thống) — dùng thoải mái.

## pink_ink.mp4 (1080×600, 2.2s, 25fps, h264 yuv420p)

Dải khói/mực hồng-tím (#C05FC0 → #D98AD3) cuộn hình chữ S quét ngang trên **nền đen thuần**,
fade in/out 0.35s ở 2 đầu — dùng để **blend screen** lên video (nền đen tự biến mất).

Tự sinh 100% bằng ffmpeg 8.0 (nguồn `perlin` + geq + lutrgb), 1 lệnh duy nhất:

```
ffmpeg -f lavfi -i "perlin=s=1000x300:r=25:octaves=5:persistence=0.55:xscale=2:yscale=2.5:tscale=0.35:seed=7:random_mode=seed" \
  -vf "format=gray,crop=540:300:x='(iw-ow)*t/2.2':y=0,\
geq=lum='clip((p(X,Y)-88)*2.6,0,255)*exp(-pow((Y-(H/2+H*0.26*sin(X/W*5+T*1.8-2.2)))/(H*0.22),2))',\
scale=1080:600,gblur=sigma=8,lutyuv=y='clip(val*1.5,0,255)',format=rgb24,\
lutrgb=r='val*(192+25*val/255)/255':g='val*(95+43*val/255)/255':b='val*(192+19*val/255)/255',\
gblur=sigma=3,fade=t=in:st=0:d=0.35,fade=t=out:st=1.85:d=0.35,format=yuv420p" \
  -t 2.2 -c:v libx264 -crf 18 -preset medium pink_ink.mp4
```

Giải thích nhanh: perlin noise (khói) → crop cửa sổ trượt ngang (khói quét từ trái sang phải) →
geq tăng tương phản + mặt nạ dải hình sin (đường cong chữ S, tâm dải lượn theo X và thời gian) →
phóng to + blur (mềm như sương) → tô màu hồng-tím theo độ sáng (tối = đen, sáng = #D98AD3) → fade.

License: tự sinh bằng ffmpeg — không vấn đề bản quyền.

## Ghi chú tích hợp

- **Thiếu file nào thì render tự bỏ qua trang trí đó** — không lỗi, không cần đủ cả 2 file.
- Muốn thay sticker/khói riêng: **cứ ghi đè file cùng tên** (`meme_cat.png` cần nền trong suốt;
  `pink_ink.mp4` cần nền đen thuần vì được blend screen).
