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
