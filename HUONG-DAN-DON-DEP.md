# Module Dọn dẹp — video.quanlyhomestay.com/unixstay

App checklist dọn phòng + chấm công cho cô dọn dẹp theo giờ, chạy chung binary
với công cụ video trong repo này.

| | |
|---|---|
| Quản lý & cô dọn dẹp | `https://video.quanlyhomestay.com/unixstay` |
| Công cụ video | `https://video.quanlyhomestay.com` (không đổi gì) |

Cùng một địa chỉ cho cả hai vai — backend tự đưa về màn đúng theo vai đăng nhập.

Dùng **đường dẫn con** `/unixstay` chứ không phải tên miền phụ, để đi chung
tunnel Cloudflare sẵn có của `video.quanlyhomestay.com` — không phải khai báo
route mới. Muốn tách sang `admin.quanlyhomestay.com` sau này thì chỉ cần thêm
route tunnel; backend đã nhận sẵn mọi tên miền bắt đầu bằng `admin.`.

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

Mở `http://localhost:8080/unixstay`.

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
   `api.dayladau.com`. Mẫu checklist và hướng dẫn vào nhà sửa tay; lần đồng bộ
   sau **không ghi đè** các mục đã sửa.
4. **Đồng bộ lịch** ở tab *Ca dọn* — đọc iCal của từng phòng và tạo ca cho mọi
   lượt khách trả phòng trong 14 ngày tới. Hệ gợi ý cô phụ trách theo khu vực,
   quản lý đổi được.
5. **Cô mở checklist**, chụp ảnh từng mục. Ảnh gửi lên ngay khi chụp, không có
   nút Lưu.
6. **Đủ ảnh bắt buộc → ca tự ghi nhận xong**, chuyển sang *Chờ đối soát*. Cô
   không phải bấm thêm nút nào.
7. **Quản lý duyệt ảnh** ở tab *Duyệt ảnh*: xem qua rồi Duyệt, hoặc Trả lại kèm
   lý do. Cô đọc được lý do trên điện thoại.
8. **Theo dõi** ở tab *Báo cáo* (số ca, số phòng, thời gian trung bình) và tab
   *Đánh giá khách*.

### Phần mềm này KHÔNG tính lương

Lương của cô dọn dẹp tính theo cơ chế riêng ở ngoài (lương cứng + thưởng review +
thưởng ngoài). Ở đây chỉ ghi nhận công việc đã làm và đo hiệu suất:

- **số ca dọn** — mỗi lượt khách trả phòng là một ca dọn kỹ
- **số phòng** khác nhau đã dọn
- **thời gian dọn trung bình** — tính từ lúc bấm Bắt đầu tới lúc đủ ảnh
- **số ca xong sau giờ khách vào**

Ca kéo dài quá 8 tiếng bị loại khỏi trung bình: gần như chắc chắn là quên bấm
Bắt đầu từ hôm trước, và một bản ghi hỏng đủ để kéo lệch cả báo cáo tháng.

### Mỗi lượt khách một ca — không phải mỗi ngày một ca

59/60 phòng của Dayladau cho thuê theo giờ, nên một phòng có thể có nhiều lượt
khách trong cùng một ngày và mỗi lượt phải dọn kỹ riêng. Khoá chống trùng vì thế
là **mã lượt đặt** lấy từ iCal, không phải cặp (phòng, ngày).

Hạn dọn xong: giờ khách sau nhận phòng, nhưng **không bao giờ ngắn hơn `clean_time`**
(đệm dọn dẹp cấu hình ở listing bên Dayladau, tối thiểu 1h). Ép hạn sát hơn đệm là
đặt ra một mốc không ai làm kịp.

---

## Thật / chưa thật

**Thật:**

- Danh sách phòng, địa chỉ, quận, số phòng ngủ, giờ nhận-trả phòng, chủ nhà —
  lấy từ `api.dayladau.com/v1/listings`, cùng endpoint công cụ video đang dùng.
- Tài khoản, mật khẩu (bcrypt), phiên đăng nhập, phân quyền.
- Ảnh checklist lưu xuống đĩa, chấm công, bảng công, CSV.

- **Lịch đặt phòng** — đọc từ feed iCal `/v1/listings/{id}/ical`, chính feed host
  dán sang Airbnb/Booking. Công khai, không cần token. Lượt `Blocked by dayladau`
  (chủ nhà tự khoá, không có khách) bị loại — dọn ở đó là giao việc không tồn tại.
- **Đánh giá của khách** — `/v1/listings/{id}/reviews`, công khai. Có điểm
  `cleanliness` riêng từng review.

**Giới hạn còn lại:**

- iCal chỉ cho **ngày**, không cho **giờ**. Với phòng thuê theo giờ, hai lượt
  khách trong cùng một ngày không phân biệt được giờ nào; giờ hiển thị là giờ trả
  phòng chuẩn của căn, quản lý sửa lại được trên màn điều phối. Muốn giờ chính
  xác thì phải đấu `/v2/calendars/search` kèm access_token của tài khoản host —
  ghi chú đã để sẵn trong `backend/hk_ical.go`.

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

Test cho module này: đọc iCal (bỏ lượt khoá lịch, đệm dọn dẹp là sàn cứng), chỉ
số hiệu suất, lọc review liên quan dọn dẹp, điều kiện đủ ảnh, phân quyền (cô
không xem được ca của cô khác), ảnh giả bị loại, dữ liệu sống sót sau khởi động
lại, đồng bộ hai lần không đẻ ca trùng nhưng hai lượt khách cùng ngày ra hai ca.

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
├── hk_ical.go       đọc lịch đặt phòng từ feed iCal
├── hk_reviews.go    đánh giá của khách + lọc phần liên quan dọn dẹp
├── hk_api.go       HTTP handlers
├── hk_seed.go      mẫu checklist khởi tạo
└── hk_*_test.go

frontend-admin/
├── src/api.js              lớp gọi API duy nhất
├── src/auth.jsx            phiên đăng nhập
├── src/router.jsx          router tối giản (~60 dòng, không thư viện ngoài)
├── src/admin/AdminPages.jsx    7 màn quản lý
├── src/cleaner/CleanerPages.jsx 3 màn của cô dọn dẹp
└── src/styles.css
```
