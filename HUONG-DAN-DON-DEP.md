# Module Dọn dẹp — admin.quanlyhomestay.com

App checklist dọn phòng + chấm công cho cô dọn dẹp theo giờ, chạy chung binary
với công cụ video trong repo này.

| | |
|---|---|
| Quản lý | `https://admin.quanlyhomestay.com` |
| Cô dọn dẹp | cùng địa chỉ — backend tự đưa về màn của cô theo vai |
| Công cụ video | `https://video.quanlyhomestay.com` (không đổi gì) |

---

## Chạy trên máy

```bash
# 1. Build app quản lý (lần đầu và mỗi khi sửa giao diện)
cd frontend-admin
npm install
npm run build          # → backend/dist-admin

# 2. Chạy backend
cd ../backend
go run . --web --port 8080
```

Mở `http://localhost:8080/admin`.

Local không có tên miền phụ nên app nằm dưới `/admin`. Trên máy chủ thật, backend
nhận ra `admin.` ở đầu tên miền và phục vụ ngay ở gốc `/`.

### Sửa giao diện (chạy song song, không phải build lại mỗi lần)

```bash
cd frontend-admin && npm run dev     # cổng 5174, tự proxy /api sang 8080
```

Cổng 5174 để không đụng công cụ video (5173) — sửa app này không phải tắt app kia.

### Nếu `npm run build` báo thiếu `rolldown-binding`

Lỗi của npm với optional dependency, không phải lỗi code:

```bash
npm install --no-save @rolldown/binding-$(node -p "process.platform+'-'+process.arch")@$(node -p "require('./node_modules/rolldown/package.json').version")
```

Dùng `--no-save` vì gói này phụ thuộc hệ điều hành — ghi vào `package.json` sẽ
làm hỏng build trên máy chủ Linux.

---

## Tài khoản quản lý đầu tiên

Lần chạy đầu, backend tự tạo một tài khoản quản lý và **in mật khẩu ra console
đúng một lần**. Đặt trước cho chủ động:

```bash
HK_ADMIN_PHONE=0912345678 HK_ADMIN_PASSWORD=mat-khau-cua-ban go run . --web
```

Không có mật khẩu mặc định kiểu `admin123` — web này nằm sau tên miền công khai.

Biến môi trường:

| Biến | Mặc định | Việc |
|---|---|---|
| `HK_DATA_DIR` | `hk_data` | Nơi chứa `housekeeping.db` và thư mục `photos/` |
| `HK_ADMIN_PHONE` | `0900000000` | SĐT tài khoản quản lý đầu tiên |
| `HK_ADMIN_PASSWORD` | sinh ngẫu nhiên | Mật khẩu tài khoản đó |

---

## Luồng chạy

1. **Cô đăng ký** bằng số điện thoại và tự đặt mật khẩu → tài khoản ở trạng thái
   *Chờ duyệt*, chưa đăng nhập được.
2. **Quản lý duyệt** ở tab *Cô dọn dẹp*.
3. **Quản lý đồng bộ phòng** ở tab *Phòng* — kéo listing thật từ
   `api.dayladau.com`. Đơn giá khoán, mẫu checklist, hướng dẫn vào nhà sửa tay;
   lần đồng bộ sau **không ghi đè** các mục đã sửa.
4. **Tạo ca dọn** ở tab *Ca dọn* (xem giới hạn bên dưới), rồi xếp cô phụ trách.
   Hệ gợi ý theo khu vực, quản lý đổi được.
5. **Cô mở checklist**, chụp ảnh từng mục. Ảnh gửi lên ngay khi chụp, không có
   nút Lưu.
6. **Đủ ảnh bắt buộc → tự chấm công.** Ca chuyển *Chờ đối soát*, tiền hiện ngay
   trong mục "Công của tôi" của cô. Cô không phải bấm thêm nút nào.
7. **Quản lý hậu kiểm** ở tab *Đối soát công*: xem ảnh rồi Duyệt, hoặc trừ tiền
   kèm lý do, hoặc Từ chối. Cô đọc được lý do trên điện thoại.
8. **Cuối tháng** mở tab *Bảng công*, xuất CSV cho kế toán.

### Cách tính công

```
công một ca = đơn giá khoán − trừ + tổng phụ cấp ĐÃ DUYỆT
tính tiền khi ca ở trạng thái: chờ đối soát HOẶC đã duyệt
```

- Ca *chờ đối soát* đã vào cột **tạm tính** — cô làm xong là thấy tiền ngay,
  không phải chờ hết tuần.
- Phụ cấp cô tự khai chỉ cộng vào tổng **sau khi quản lý duyệt**. Trước đó nó nằm
  riêng ở cột "chờ duyệt" để không ai hiểu nhầm là tiền đã có.
- Trừ không bao giờ vượt quá tiền khoán — công không âm.

Toàn bộ phép tính nằm ở `backend/hk_model.go` và **chỉ ở đó**. Frontend không tự
cộng lại; nếu để React tính song song thì sớm muộn hai bên lệch nhau, và bên sai
luôn là bên cô dọn dẹp nhìn thấy.

---

## Thật / chưa thật

**Thật:**

- Danh sách phòng, địa chỉ, quận, số phòng ngủ, giờ nhận-trả phòng, chủ nhà —
  lấy từ `api.dayladau.com/v1/listings`, cùng endpoint công cụ video đang dùng.
- Tài khoản, mật khẩu (bcrypt), phiên đăng nhập, phân quyền.
- Ảnh checklist lưu xuống đĩa, chấm công, bảng công, CSV.

**Chưa thật — cần bạn cung cấp để hoàn thiện:**

- **Lịch check-out từng ngày.** Endpoint listing ở trên là API *tìm phòng* (phòng
  nào còn trống), không phải lịch đặt. Muốn biết "hôm nay phòng nào khách trả"
  phải gọi API reservations của host — endpoint đó cần xác thực và hiện chưa có
  token. Chỗ cắm vào đã chừa sẵn ở `hkFetchCheckouts()` trong
  `backend/hk_sync.go`; thay thân hàm đó là xong, phần còn lại không phải sửa.

  Trong lúc chờ, quản lý bấm **“+ Thêm ca”** ở tab *Ca dọn* để tạo ca thủ công.

---

## Lưu dữ liệu

SQLite (`modernc.org/sqlite`, thuần Go, không cần cgo) — một file, không phải cài
server database:

```
hk_data/
├── housekeeping.db     ← tài khoản, phòng, ca, chấm công
└── photos/             ← ảnh checklist
```

**Sao lưu = copy cả thư mục `hk_data/`.** Ảnh checklist là chứng từ chấm công,
nên đặt vòng đời lưu trữ tối thiểu 12 tháng.

Ảnh hiện nằm trên đĩa máy chủ. Muốn đẩy lên Cloudflare R2 của Dayladau thì chỉ
phải đổi `handlePhotoUpload` trong `backend/hk_api.go` — chỗ khác không đụng.

---

## Bảo mật

- Mật khẩu băm bcrypt, không bao giờ trả về frontend.
- Token 30 ngày; khoá tài khoản là token cũ **mất tác dụng ngay**, không đợi hết hạn.
- Cô dọn dẹp chỉ đọc/ghi được ca của chính mình — ép ở backend, không tin tham số
  frontend gửi lên.
- Ảnh checklist **cần đăng nhập mới xem được**: ảnh chụp bên trong nhà khách.
- Chỉ nhận ảnh do chính server cấp phát; URL từ tên miền ngoài bị loại, để không
  ai dán ảnh bất kỳ làm "bằng chứng đã dọn".
- Kiểu file nhận diện bằng nội dung thật, không tin đuôi tên hay `Content-Type`.

---

## Kiểm thử

```bash
cd backend && go test .
```

52 test cho module này: luật tính công, điều kiện đủ ảnh, phân quyền (cô không
xem được ca của cô khác, không tự duyệt được tiền của mình), ảnh giả bị loại, dữ
liệu sống sót sau khi khởi động lại.

Hai test `TestRenderListingOverlayPNG_*` cần ImageMagick (`brew install
imagemagick`); chúng thuộc công cụ video, không liên quan module này.

---

## File

```
backend/
├── hk_model.go     luật nghiệp vụ thuần (tính công, tiến độ) — không đụng DB/HTTP
├── hk_store.go     SQLite; đổi sang Postgres/Firebase chỉ phải sửa file này
├── hk_auth.go      bcrypt, token, phân quyền
├── hk_sync.go      đồng bộ phòng từ Dayladau + tạo ca
├── hk_api.go       HTTP handlers
├── hk_seed.go      mẫu checklist khởi tạo
└── hk_*_test.go

frontend-admin/
├── src/api.js              lớp gọi API duy nhất
├── src/auth.jsx            phiên đăng nhập
├── src/router.jsx          router tối giản (~60 dòng, không thư viện ngoài)
├── src/admin/AdminPages.jsx    6 màn quản lý
├── src/cleaner/CleanerPages.jsx 3 màn của cô dọn dẹp
└── src/styles.css
```
