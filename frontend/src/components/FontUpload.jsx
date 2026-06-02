import { useState, useEffect, useRef, useCallback } from 'react'
import { uploadFont, listFonts, deleteFont, fontPreviewUrl } from '../api/client'

// FontUpload lets the user bring their own title font. Uploads are validated +
// (if variable) auto-instanced server-side; the preview is rendered through the
// REAL gg pipeline so what's shown is what gen-video / the thumbnail produces —
// a browser-rendered mockup would hide the thin/tofu failure modes.
export default function FontUpload({ value = '', onChange, previewText }) {
  const [fonts, setFonts] = useState([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const fileRef = useRef(null)

  const refresh = useCallback(() => {
    listFonts().then(setFonts).catch(e => setErr(e.message || 'Không tải được danh sách font'))
  }, [])
  useEffect(() => { refresh() }, [refresh])

  const handleFile = async (file) => {
    if (!file) return
    setBusy(true); setErr('')
    try {
      const { data } = await uploadFont(file)
      if (data.error) throw new Error(data.error)
      await refresh()
      onChange?.(data.font.file) // auto-select the freshly uploaded font
    } catch (e) {
      setErr(e?.response?.data?.error || e.message || 'Tải font lỗi')
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const handleDelete = async (file, e) => {
    e.stopPropagation()
    try {
      await deleteFont(file)
      if (value === file) onChange?.('')
      refresh()
    } catch (_) { /* ignore */ }
  }

  const selected = fonts.find(f => f.file === value)
  const sampleText = (previewText || '').trim() || 'Biệt thự 3 tầng – 120m²'

  return (
    <div className="field">
      <label className="flabel">Phông chữ tiêu đề (tự tải lên)</label>

      <div className="font-chips">
        <button
          type="button"
          className={`font-chip${!value ? ' on' : ''}`}
          onClick={() => onChange?.('')}
        >
          Mặc định (theo template)
        </button>
        {fonts.map(f => (
          <button
            type="button"
            key={f.file}
            className={`font-chip${value === f.file ? ' on' : ''}`}
            title={f.warnings?.join(' · ') || f.label}
            onClick={() => onChange?.(f.file)}
          >
            {!f.vietnamese_ok && <span className="font-warn" title="Thiếu glyph tiếng Việt">⚠</span>}
            {f.label}
            {f.instanced && <span className="font-badge" title="Font variable đã được tạo bản static đậm">↳static</span>}
            {f.source === 'uploaded' && (
              <span className="font-x" title="Xoá" onClick={(e) => handleDelete(f.file, e)}>✕</span>
            )}
          </button>
        ))}
      </div>

      <div style={{ marginTop: 8 }}>
        <input
          ref={fileRef}
          type="file"
          accept=".ttf,.otf"
          style={{ display: 'none' }}
          onChange={e => handleFile(e.target.files?.[0])}
        />
        <button
          type="button"
          className="seg-btn"
          disabled={busy}
          onClick={() => fileRef.current?.click()}
        >
          {busy ? '⏳ Đang xử lý…' : '⬆ Tải font .ttf / .otf'}
        </button>
      </div>

      {err && <div style={{ fontSize: 11, color: 'var(--red)', marginTop: 6 }}>{err}</div>}

      {value && selected && (
        <div className="font-preview-box" style={{ marginTop: 10 }}>
          <img
            className="font-preview-img"
            src={fontPreviewUrl(value, sampleText)}
            alt="preview"
          />
          {!selected.vietnamese_ok && (
            <div style={{ fontSize: 11, color: 'var(--red)', marginTop: 6 }}>
              ⚠ Font này thiếu glyph tiếng Việt — chữ có dấu sẽ ra ô vuông (xem preview).
            </div>
          )}
          {selected.instanced && (
            <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 4, fontStyle: 'italic' }}>
              Font variable đã được tạo bản static đậm tự động để render đúng độ dày.
            </div>
          )}
        </div>
      )}

      <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 6, fontStyle: 'italic' }}>
        Áp cho tiêu đề video &amp; thumbnail. Preview render bằng đúng engine của video — thấy sao ra vậy.
      </div>
    </div>
  )
}
