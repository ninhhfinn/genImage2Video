import { useState, useEffect, useCallback } from 'react'
import { fetchScript, getScripts, likeScript, deleteScript } from '../api/client'

// ScriptPanel — xem/sửa kịch bản thuyết minh TRƯỚC khi render + thư viện kịch
// bản đã lưu (bấm ❤️ để bản đó thành ví dụ few-shot dạy Claude các lần sau).
//
// Luồng: "Xem kịch bản" gọi POST /api/script (cùng cache với render nên duyệt
// xong bấm render KHÔNG tốn thêm lượt Claude). Sửa tay → "Dùng bản này" → App
// gửi kèm `script` trong request render.
export default function ScriptPanel({ buildScriptReq, canFetch, approvedScript, onApprove, toast }) {
  const [segments, setSegments] = useState(null)   // bản đang xem/sửa
  // Tiêu đề ghim (hook) 2 dòng + cụm nhấn màu — script cũ không có → ''
  const [hook, setHook]         = useState({ line1: '', line2: '', emphasis: '' })
  // Lời hook đọc trên cảnh đi đường (intro) — chỉ có khi bật intro clip.
  const [intro, setIntro]       = useState({ narration: '', caption: '', enabled: false })
  const [hookWarning, setHookWarning] = useState('') // giá hook lệch dữ liệu listing
  const [warnings, setWarnings] = useState([])       // từ cấm / ngôn ngữ quảng cáo
  const [wordBudget, setBudget] = useState(0)
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState('')
  const [showLib, setShowLib]   = useState(false)
  const [lib, setLib]           = useState([])

  const load = useCallback(async (force) => {
    setLoading(true); setError('')
    try {
      const { data } = await fetchScript({ ...buildScriptReq(), force: !!force })
      if (data.error) throw new Error(data.error)
      setSegments(data.segments || [])
      setHook({
        line1:    data.hook_line1    || '',
        line2:    data.hook_line2    || '',
        emphasis: data.hook_emphasis || '',
      })
      setIntro({
        narration: data.intro_narration || '',
        caption:   data.intro_caption   || '',
        enabled:   !!data.intro_enabled,
      })
      setHookWarning(data.hook_warning || '')
      setWarnings(data.warnings || [])
      setBudget(data.word_budget || 0)
      onApprove(null) // bản mới sinh — chờ user duyệt lại
      if (force) toast('Đã viết lại kịch bản ✓', 'ok')
    } catch (e) {
      setError(e?.response?.data?.error || e.message)
      toast('Lỗi viết kịch bản', 'err')
    } finally {
      setLoading(false)
    }
  }, [buildScriptReq, onApprove, toast])

  const refreshLib = useCallback(async () => {
    try { setLib(await getScripts()) } catch { /* ignore */ }
  }, [])
  useEffect(() => { if (showLib) refreshLib() }, [showLib, refreshLib])

  const editSeg = (i, patch) =>
    setSegments(segs => segs.map((s, j) => (j === i ? { ...s, ...patch } : s)))

  const words = (t) => (t || '').trim().split(/\s+/).filter(Boolean).length
  const limitFor = (i) =>
    wordBudget > 0 ? (i === 0 || i === (segments?.length || 1) - 1 ? wordBudget * 2 : wordBudget) : 0

  const toggleLike = async (e) => {
    try {
      await likeScript(e.id, !e.liked)
      setLib(l => l.map(x => (x.id === e.id ? { ...x, liked: !e.liked } : x)))
      toast(!e.liked ? '❤️ Sẽ dùng làm mẫu dạy Claude các lần sau' : 'Đã bỏ khỏi bộ mẫu', 'ok')
    } catch { toast('Lỗi cập nhật', 'err') }
  }
  const removeEntry = async (e) => {
    try {
      await deleteScript(e.id)
      setLib(l => l.filter(x => x.id !== e.id))
    } catch { toast('Lỗi xoá', 'err') }
  }

  return (
    <div className="field" style={{ padding: '12px 14px 4px' }}>
      <label className="flabel">📜 Kịch bản thuyết minh</label>

      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 8 }}>
        <button className="btn btn-outline btn-sm" disabled={!canFetch || loading} onClick={() => load(false)}>
          {loading ? '⏳ Claude đang viết…' : segments ? '🔄 Tải lại' : '📜 Xem kịch bản'}
        </button>
        {segments && (
          <button className="btn btn-outline btn-sm" disabled={loading} onClick={() => load(true)}>
            🎲 Viết lại
          </button>
        )}
        {segments && (
          approvedScript
            ? <button className="btn btn-green btn-sm" onClick={() => onApprove(null)}>✔ Đang dùng bản này — bấm để bỏ</button>
            : <button className="btn btn-gold btn-sm" onClick={() => onApprove({
                segments,
                hook_line1:    hook.line1,
                hook_line2:    hook.line2,
                hook_emphasis: hook.emphasis,
                intro_narration: intro.narration,
                intro_caption:   intro.caption,
              })}>✔ Dùng bản này khi render</button>
        )}
      </div>
      {!canFetch && <div className="hint" style={{ opacity: 0.7 }}>Upload ảnh / chọn listing trước đã.</div>}
      {error && <div className="hint" style={{ color: 'var(--red, #e5484d)' }}>{error}</div>}

      {/* ── Cảnh báo từ cấm / ngôn ngữ quảng cáo (soft, không chặn render) ── */}
      {segments && warnings.length > 0 && (
        <div className="hint" style={{ color: 'var(--red, #e5484d)', marginBottom: 8 }}>
          {warnings.map((wmsg, i) => <div key={i}>⚠️ {wmsg}</div>)}
        </div>
      )}

      {/* ── Hook cảnh đi đường (intro) — đọc trên video quay thật mở đầu ── */}
      {segments && intro.enabled && (
        <div style={{ marginBottom: 10 }}>
          <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 3 }}>
            <span className="hint" style={{ fontWeight: 600, opacity: 0.85 }}>🎬 Hook cảnh đi đường (đọc trên video)</span>
            <span className="hint" style={{ opacity: 0.7, color: words(intro.narration) > 22 ? 'var(--red, #e5484d)' : undefined }}>
              {words(intro.narration)}/~20 từ
            </span>
          </div>
          <textarea
            className="inp body-font"
            value={intro.narration}
            onChange={e => setIntro(v => ({ ...v, narration: e.target.value }))}
            rows={2}
            placeholder="1-2 câu hook đọc trên cảnh đi đường (số viết thành CHỮ)"
            style={{ width: '100%', resize: 'vertical', fontSize: 12, lineHeight: 1.45 }}
          />
        </div>
      )}

      {/* ── Tiêu đề ghim (hook) — 2 dòng hiện cố định trên video + cụm nhấn màu ── */}
      {segments && (
        <div style={{ marginBottom: 10 }}>
          <div className="hint" style={{ fontWeight: 600, opacity: 0.85, marginBottom: 4 }}>📌 Tiêu đề ghim (hook)</div>
          {hookWarning && (
            <div className="hint" style={{ color: 'var(--red, #e5484d)', marginBottom: 4 }}>
              ⚠️ {hookWarning}
            </div>
          )}
          {[
            ['line1',    'Dòng 1',            true],
            ['line2',    'Dòng 2 (chứa giá)', true],
            ['emphasis', 'Cụm nhấn màu',      false],
          ].map(([k, label, limited]) => {
            const val = hook[k] || ''
            const over = limited && val.length > 28
            return (
              <div key={k} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 4 }}>
                <span className="hint" style={{ flex: '0 0 auto', width: 110, opacity: 0.75 }}>{label}</span>
                <input
                  className="inp body-font"
                  value={val}
                  onChange={e => setHook(h => ({ ...h, [k]: e.target.value }))}
                  placeholder={label}
                  style={{ flex: 1, marginBottom: 0, fontSize: 12, padding: '5px 8px' }}
                />
                {limited && (
                  <span className="hint" style={{ opacity: 0.7, whiteSpace: 'nowrap', color: over ? 'var(--red, #e5484d)' : undefined }}>
                    {val.length}/28
                  </span>
                )}
              </div>
            )
          })}
        </div>
      )}

      {segments && segments.map((s, i) => {
        const w = words(s.narration)
        const lim = limitFor(i)
        const over = lim > 0 && w > lim
        return (
          <div key={i} style={{ marginBottom: 8 }}>
            <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 3 }}>
              <input
                className="inp body-font"
                value={s.caption || ''}
                onChange={e => editSeg(i, { caption: e.target.value })}
                style={{ width: 130, marginBottom: 0, fontSize: 11, padding: '5px 8px' }}
                placeholder={`Cảnh ${i + 1}`}
              />
              <span className="hint" style={{ opacity: 0.7, color: over ? 'var(--red, #e5484d)' : undefined }}>
                {w}{lim ? `/${lim}` : ''} từ{i === 0 ? ' · hook' : i === segments.length - 1 ? ' · CTA' : ''}
              </span>
            </div>
            <textarea
              className="inp body-font"
              value={s.narration || ''}
              onChange={e => editSeg(i, { narration: e.target.value })}
              rows={2}
              style={{ width: '100%', resize: 'vertical', fontSize: 12, lineHeight: 1.45 }}
            />
          </div>
        )
      })}
      {segments && (
        <div className="hint" style={{ opacity: 0.6, marginBottom: 6 }}>
          Sửa xong nhớ bấm "Dùng bản này" — số và giá viết thành CHỮ để giọng đọc không đọc sai.
        </div>
      )}

      {/* ── Thư viện kịch bản đã lưu ── */}
      <button className="btn btn-outline btn-sm" style={{ marginBottom: 6 }} onClick={() => setShowLib(v => !v)}>
        📚 Kịch bản đã lưu {showLib ? '▲' : '▼'}
      </button>
      {showLib && (
        lib.length === 0
          ? <div className="hint" style={{ opacity: 0.6 }}>Chưa có — render video thuyết minh xong sẽ tự lưu vào đây.</div>
          : lib.map(e => (
            <div key={e.id} style={{ display: 'flex', gap: 6, alignItems: 'center', padding: '4px 0', borderBottom: '1px solid var(--border2)' }}>
              <button
                type="button"
                title={e.liked ? 'Bỏ khỏi bộ mẫu dạy Claude' : 'Đánh dấu HAY → thành mẫu dạy Claude'}
                onClick={() => toggleLike(e)}
                style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 14, filter: e.liked ? 'none' : 'grayscale(1) opacity(.45)' }}
              >❤️</button>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 12, fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {e.nickname || e.listing_id || '(không tên)'}
                  <span className="hint" style={{ marginLeft: 6, opacity: 0.6 }}>
                    {e.persona === 'lichsu' ? '🤵' : '😜'}{e.edited ? ' · đã sửa tay' : ''}
                  </span>
                </div>
                <div className="hint" style={{ opacity: 0.6, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {e.segments?.[0]?.narration || ''}
                </div>
              </div>
              <button type="button" title="Xoá" onClick={() => removeEntry(e)}
                style={{ background: 'none', border: 'none', cursor: 'pointer', opacity: 0.55 }}>🗑</button>
            </div>
          ))
      )}
    </div>
  )
}
