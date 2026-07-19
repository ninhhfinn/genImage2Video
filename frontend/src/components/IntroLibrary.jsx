import { useState, useEffect, useRef, useCallback } from 'react'
import {
  uploadIntro, getIntros, deleteIntro,
  importIntroDrive, getIntroImportStatus,
} from '../api/client'

// IntroLibrary quản lý thư viện clip "cảnh đi đường" cho mode Thuyết minh AI.
// KHÁC MusicPicker: không chọn 1 clip — backend tự bốc ngẫu nhiên mỗi lần render.
// Ở đây chỉ upload / nhập từ Drive / xoá. Thư viện rỗng → render tự bỏ intro.
export default function IntroLibrary() {
  const [clips, setClips] = useState([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [driveUrl, setDriveUrl] = useState('')
  const [imp, setImp] = useState(null) // trạng thái import Drive
  const fileRef = useRef(null)
  const pollRef = useRef(null)

  const refresh = useCallback(() => {
    getIntros().then(setClips).catch(e => setErr(e.message || 'Không tải được danh sách clip'))
  }, [])
  useEffect(() => { refresh() }, [refresh])
  useEffect(() => () => { if (pollRef.current) clearInterval(pollRef.current) }, [])

  const handleFile = async (file) => {
    if (!file) return
    setBusy(true); setErr('')
    try {
      const { data } = await uploadIntro(file)
      if (data.error) throw new Error(data.error)
      await refresh()
    } catch (e) {
      setErr(e?.response?.data?.error || e.message || 'Tải clip lỗi')
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const handleDelete = async (file, e) => {
    e.stopPropagation()
    try { await deleteIntro(file); refresh() } catch (_) { /* ignore */ }
  }

  const pollImport = useCallback(() => {
    if (pollRef.current) clearInterval(pollRef.current)
    pollRef.current = setInterval(async () => {
      try {
        const s = await getIntroImportStatus()
        setImp(s)
        if (!s.running) {
          clearInterval(pollRef.current); pollRef.current = null
          refresh()
        }
      } catch (_) { /* ignore */ }
    }, 2000)
  }, [refresh])

  const handleImport = async () => {
    const url = driveUrl.trim()
    if (!url) return
    setErr('')
    try {
      const { data } = await importIntroDrive(url)
      if (data.error) throw new Error(data.error)
      setImp({ running: true, total: 0, done: 0, imported: [], errors: [] })
      pollImport()
    } catch (e) {
      setErr(e?.response?.data?.error || e.message || 'Nhập Drive lỗi')
    }
  }

  const importing = imp?.running

  return (
    <div className="field" style={{ marginBottom: 8 }}>
      <label className="flabel">🎬 Thư viện cảnh đi đường ({clips.length} clip)</label>

      {clips.length === 0 && (
        <div style={{ fontSize: 11, color: 'var(--red)', marginBottom: 6 }}>
          Chưa có clip nào — video sẽ render KHÔNG có đoạn intro. Tải lên hoặc nhập từ Drive bên dưới.
        </div>
      )}

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        {clips.map(c => (
          <div key={c.file} style={{ position: 'relative', width: 84 }}>
            <video
              src={`/api/intro-file?name=${encodeURIComponent(c.file)}#t=0.6`}
              muted
              playsInline
              preload="metadata"
              title={c.label}
              onMouseEnter={e => { e.currentTarget.play?.().catch(() => {}) }}
              onMouseLeave={e => { e.currentTarget.pause?.(); e.currentTarget.currentTime = 0.6 }}
              style={{
                width: 84, height: 149, objectFit: 'cover', borderRadius: 8,
                background: '#000', display: 'block', cursor: 'pointer',
              }}
            />
            <span
              className="font-x"
              title="Xoá"
              onClick={(e) => handleDelete(c.file, e)}
              style={{
                position: 'absolute', top: 2, right: 2, background: 'rgba(0,0,0,.6)',
                color: '#fff', borderRadius: '50%', width: 18, height: 18,
                lineHeight: '18px', textAlign: 'center', fontSize: 11, cursor: 'pointer',
              }}
            >✕</span>
          </div>
        ))}
      </div>

      <div style={{ marginTop: 8 }}>
        <input
          ref={fileRef}
          type="file"
          accept="video/mp4,video/quicktime,.mp4,.mov,.m4v,.webm"
          style={{ display: 'none' }}
          onChange={e => handleFile(e.target.files?.[0])}
        />
        <button
          type="button"
          className="seg-btn"
          disabled={busy || importing}
          onClick={() => fileRef.current?.click()}
        >
          {busy ? '⏳ Đang tải + chuẩn hoá…' : '⬆ Tải clip .mp4 / .mov / .m4v / .webm'}
        </button>
      </div>

      <div style={{ marginTop: 8, display: 'flex', gap: 6 }}>
        <input
          type="text"
          value={driveUrl}
          onChange={e => setDriveUrl(e.target.value)}
          placeholder="Dán link Google Drive (folder hoặc file)…"
          style={{ flex: 1, minWidth: 0 }}
          disabled={importing}
        />
        <button
          type="button"
          className="seg-btn"
          disabled={importing || !driveUrl.trim()}
          onClick={handleImport}
        >
          {importing ? '⏳ Đang nhập…' : '↧ Nhập từ Drive'}
        </button>
      </div>

      {imp && (
        <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 6 }}>
          {imp.running
            ? `Đang tải & chuẩn hoá… ${imp.done}/${imp.total || '?'} clip`
            : `Xong: đã nhập ${imp.imported?.length || 0} clip`}
          {imp.errors?.length > 0 && (
            <div style={{ color: 'var(--red)' }}>
              {imp.errors.slice(0, 3).map((m, i) => <div key={i}>⚠ {m}</div>)}
            </div>
          )}
        </div>
      )}

      {err && <div style={{ fontSize: 11, color: 'var(--red)', marginTop: 6 }}>{err}</div>}

      <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 6, fontStyle: 'italic' }}>
        Mỗi clip tự chuẩn hoá về dọc 1080×1920. Mỗi lần render bốc ngẫu nhiên 1 clip (tránh lặp clip vừa dùng).
      </div>
    </div>
  )
}
