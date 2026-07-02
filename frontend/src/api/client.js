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

export const downloadVideo = () => {
  window.location.href = '/api/download'
}

export const exportExcel = () => {
  window.location.href = '/api/export-excel'
}
