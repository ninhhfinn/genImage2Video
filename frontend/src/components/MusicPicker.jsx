import { useState, useEffect, useRef, useCallback } from 'react'
import { uploadMusic, getMusic, deleteMusic } from '../api/client'

// MusicPicker lets the user bring their own background music for the narrated
// (AI voice-over) mode. Uploads are validated server-side; the list mirrors the
// custom-font flow — pick one chip or "🔇 Không nhạc" for silence.
export default function MusicPicker({ value = '', onChange }) {
  const [tracks, setTracks] = useState([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const fileRef = useRef(null)

  const refresh = useCallback(() => {
    getMusic().then(setTracks).catch(e => setErr(e.message || 'Không tải được danh sách nhạc'))
  }, [])
  useEffect(() => { refresh() }, [refresh])

  const handleFile = async (file) => {
    if (!file) return
    setBusy(true); setErr('')
    try {
      const { data } = await uploadMusic(file)
      if (data.error) throw new Error(data.error)
      await refresh()
      onChange?.(data.music.file) // auto-select the freshly uploaded track
    } catch (e) {
      setErr(e?.response?.data?.error || e.message || 'Tải nhạc lỗi')
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const handleDelete = async (file, e) => {
    e.stopPropagation()
    try {
      await deleteMusic(file)
      if (value === file) onChange?.('')
      refresh()
    } catch (_) { /* ignore */ }
  }

  return (
    <div className="field">
      <label className="flabel">🎵 Nhạc nền (tự tải lên)</label>

      <div className="font-chips">
        <button
          type="button"
          className={`font-chip${!value ? ' on' : ''}`}
          onClick={() => onChange?.('')}
        >
          🔇 Không nhạc
        </button>
        {tracks.map(t => (
          <button
            type="button"
            key={t.file}
            className={`font-chip${value === t.file ? ' on' : ''}`}
            title={t.label}
            onClick={() => onChange?.(t.file)}
          >
            {t.label}
            <span className="font-x" title="Xoá" onClick={(e) => handleDelete(t.file, e)}>✕</span>
          </button>
        ))}
      </div>

      <div style={{ marginTop: 8 }}>
        <input
          ref={fileRef}
          type="file"
          accept=".mp3,.m4a,.wav,.aac"
          style={{ display: 'none' }}
          onChange={e => handleFile(e.target.files?.[0])}
        />
        <button
          type="button"
          className="seg-btn"
          disabled={busy}
          onClick={() => fileRef.current?.click()}
        >
          {busy ? '⏳ Đang tải…' : '⬆ Tải nhạc .mp3 / .m4a / .wav / .aac'}
        </button>
      </div>

      {err && <div style={{ fontSize: 11, color: 'var(--red)', marginTop: 6 }}>{err}</div>}

      <div style={{ fontSize: 10, color: 'var(--muted)', marginTop: 6, fontStyle: 'italic' }}>
        Nhạc nền trộn dưới giọng thuyết minh (≤20MB). Để trống = không nhạc.
      </div>
    </div>
  )
}
