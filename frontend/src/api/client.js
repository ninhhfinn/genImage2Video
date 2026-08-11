import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

export const uploadImages = (files) => {
  const fd = new FormData()
  files.forEach(f => fd.append('images', f))
  return api.post('/upload', fd)
}

export const parseListings = (body) =>
  api.post('/parse-listings', body)

// Dayladau-only loader: server builds the v1/listings URL from date/guest params.
export const fetchDayladau = (body) =>
  api.post('/dayladau-listings', body, { timeout: 60000 })

export const selectListing = (body) =>
  api.post('/select-listing', body, { timeout: 120000 }) // many photos + retries can be slow; backend caps concurrency/attempts

export const startRender = (body) =>
  api.post('/render', body)

// Thumbnail (still collage image) — synchronous, returns { url }
export const renderThumbnail = (body) =>
  api.post('/render-thumbnail', body)

// Thumbnail từ Excel: upload file .xlsx (ảnh nhúng + cột nhãn) → random 4 ảnh + 4
// nhãn, tỉnh lấy từ address listing. Trả { url }.
// Lưu "file ảnh + nhãn" lên server (upload 1 lần, dùng lại nhiều lần).
export const saveThumbImages = (file) => {
  const fd = new FormData()
  fd.append('excel', file)
  return api.post('/save-thumb-images', fd, { timeout: 120000 })
}

// Trạng thái file ảnh đã lưu (khi mở app): { saved, images, labels }.
export const getThumbImagesStatus = () =>
  api.get('/save-thumb-images').then(r => r.data)

// Tạo thumbnail: ảnh+nhãn lấy từ file ĐÃ LƯU trên server; textFile (tuỳ chọn) =
// file chữ 2 cột (serif | script). badge override nhập tay (rỗng = auto).
export const renderExcelThumbnail = (
  { textFile = null, address = '', template = 'valentine', amenities = [], badge = '' } = {},
) => {
  const fd = new FormData()
  if (textFile) fd.append('excel_text', textFile)
  fd.append('address', address)
  fd.append('template', template)
  amenities.forEach(a => { if (a) fd.append('amenity', a) })
  fd.append('badge', badge)
  return api.post('/render-thumbnail-excel', fd, { timeout: 120000 })
}

export const getStatus = () =>
  api.get('/status')

export const getHistory = () =>
  api.get('/history')

export const getThumbnailHistory = () =>
  api.get('/thumbnail-history')

// ── Custom fonts (bring-your-own) ──
export const uploadFont = (file) => {
  const fd = new FormData()
  fd.append('font', file)
  return api.post('/upload-font', fd, { timeout: 60000 }) // instancing can be slow
}

export const listFonts = () =>
  api.get('/fonts').then(r => r.data || [])

export const deleteFont = (font) =>
  api.post('/delete-font', { font })

// Faithful gg-rendered preview PNG URL (not cached) for a font + sample text.
export const fontPreviewUrl = (font, text) =>
  `/api/font-preview?font=${encodeURIComponent(font)}&text=${encodeURIComponent(text || '')}&_=${Date.now()}`

// ── Nhạc nền (bring-your-own) — dùng cho chế độ Thuyết minh AI ──
export const uploadMusic = (file) => {
  const fd = new FormData()
  fd.append('music', file)
  return api.post('/upload-music', fd, { timeout: 60000 })
}

export const getMusic = () =>
  api.get('/music').then(r => r.data || [])

export const deleteMusic = (music) =>
  api.post('/delete-music', { music })

// ── Thư viện clip "cảnh đi đường" (intro) — dùng cho Thuyết minh AI ──
export const uploadIntro = (file) => {
  const fd = new FormData()
  fd.append('intro', file)
  return api.post('/upload-intro', fd, { timeout: 600000 }) // .MOV to + chuẩn hoá ffmpeg
}

export const getIntros = () =>
  api.get('/intros').then(r => r.data || [])

export const deleteIntro = (intro) =>
  api.post('/delete-intro', { intro })

export const importIntroDrive = (url) =>
  api.post('/import-intro-drive', { url })

export const getIntroImportStatus = () =>
  api.get('/intro-import-status').then(r => r.data)

// ── Kịch bản thuyết minh: xem/sửa trước render + thư viện dạy Claude ──
export const fetchScript = (body) =>
  api.post('/script', body, { timeout: 300000 }) // Claude viết ~30–120s

export const getScripts = () =>
  api.get('/scripts').then(r => r.data || [])

export const likeScript = (id, liked) =>
  api.post('/like-script', { id, liked })

export const deleteScript = (id) =>
  api.post('/delete-script', { id })

export const downloadVideo = () => {
  window.location.href = '/api/download'
}

export const exportExcel = () => {
  window.location.href = '/api/export-excel'
}
