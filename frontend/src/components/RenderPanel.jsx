import { useEffect, useState, useCallback, useRef } from 'react'
import { getHistory, getThumbnailHistory, downloadVideo, exportExcel } from '../api/client'

function downloadUrlToDisk(url, filename) {
  const a = document.createElement('a')
  a.href = url
  a.download = filename || 'video.mp4'
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

// ── RenderPanel ─────────────────────────────────────────────────────────
export function RenderPanel({ canRender, onRender, rendering, done, pct, progText, status, error, queue, isQueueMode,
  batchMode = 'both', canThumbnail, onThumbnail, thumbBusy, thumbUrl, thumbErr,
  onExcelThumbnail, excelBusy, excelUrl, excelErr,
  onSaveImages, savingImages, excelSavedInfo, thumbTemplate = '' }) {
  const imageInputRef = useRef(null)
  const textInputRef  = useRef(null)
  const [lastTextFile, setLastTextFile] = useState(null)
  const [vBadge, setVBadge] = useState('')  // rỗng = tự động (Quận + Tỉnh)
  const pendingCount = queue?.filter(q => q.status==='pending').length || 0
  const doneCount    = queue?.filter(q => q.status==='done').length    || 0
  const totalCount   = queue?.length || 0
  // Listing đơn: nhãn nút đổi theo chế độ xuất đã chọn (giữ nguyên cả khi disabled).
  const singleLabel = batchMode === 'video'     ? 'Tạo video'
                    : batchMode === 'thumbnail' ? 'Tạo thumbnail'
                    :                             'Tạo video + ảnh bìa'
  const idle = !rendering && !done && !error && !isQueueMode

  return (
    <div className="render-panel">
      {/* Điểm neo thị giác khi chưa render — khung 9:16 như mockup */}
      {idle && (
        <>
          <div className="reel-ph">
            <span className="fmt">1080×1920</span>
            <span className="msg">{canRender ? 'Bấm nút dưới để dựng reel' : 'Video 9:16 sẽ hiện ở đây'}</span>
          </div>
          <div className="spec-chips">
            <span>MP4 · H.264</span><span>9:16</span><span>30 fps</span>
          </div>
        </>
      )}

      <button className="btn btn-gold btn-full" onClick={onRender} disabled={!canRender||rendering}>
        {rendering
          ? <><span className="spin">⟳</span> Đang render...</>
          : isQueueMode && pendingCount > 0
            ? `Render ${pendingCount} listing`
            : isQueueMode && pendingCount === 0
              ? 'Tất cả đã xong ✓'
              : singleLabel}
      </button>
      {!canRender && !rendering && !isQueueMode && (
        <div style={{fontSize:11,color:'var(--muted)',textAlign:'center',marginTop:-6}}>
          Chọn ảnh ở cột 1 trước
        </div>
      )}

      {/* Queue progress */}
      {isQueueMode && totalCount > 0 && (
        <div style={{background:'var(--bg3)',border:'1px solid var(--border)',borderRadius:'var(--r)',padding:12}}>
          <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:8}}>
            <span style={{fontSize:11,fontWeight:600}}>Hàng chờ render</span>
            <span style={{fontFamily:'var(--mono)',fontSize:10,color:'var(--gold)'}}>{doneCount}/{totalCount}</span>
          </div>
          {/* Overall progress bar */}
          <div className="prog-track" style={{marginBottom:10}}>
            <div className="prog-fill" style={{width:(doneCount/totalCount*100)+'%',background:'var(--green)'}}/>
          </div>
          {/* Queue items */}
          <div style={{maxHeight:160,overflowY:'auto'}}>
            {queue.map((q,i) => (
              <div key={i} className="queue-item">
                <div className="queue-icon">
                  {q.status==='done'?'✅':q.status==='running'?'🎬':q.status==='err'?'❌':'⏳'}
                </div>
                <div className="queue-name">
                  {q.listing.nickname||q.listing.name||`Listing ${i+1}`}
                </div>
                <div className={`queue-status ${q.status}`}>
                  {q.status==='pending'?'Chờ':q.status==='running'?'Đang render':q.status==='done'?'Xong':'Lỗi'}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Single render progress */}
      {(rendering || done || error) && (
        <ProgressBox status={status} text={progText} pct={pct} done={done} err={error}/>
      )}

      {/* Download — only show for single mode */}
      {done && !isQueueMode && (
        <button className="btn btn-gold btn-full" onClick={downloadVideo}>
          Tải video xuống ↓
        </button>
      )}

      {/* ── Thumbnail (ảnh bìa) ── */}
      <div style={{marginTop:6,paddingTop:12,borderTop:'1px solid var(--border)'}}>
        <div className="plabel" style={{marginBottom:8}}>Thumbnail (ảnh bìa)</div>

        {thumbTemplate !== 'valentine' ? (
          /* Template thường: tạo từ ảnh đã upload + listing (giữ nguyên như cũ) */
          <>
            <button className="btn btn-outline btn-full" onClick={onThumbnail} disabled={!canThumbnail||thumbBusy}>
              {thumbBusy ? <><span className="spin">⟳</span> Đang tạo ảnh...</> : 'Tạo Thumbnail'}
            </button>
            {thumbErr && <div style={{color:'var(--red)',fontSize:11,marginTop:6}}>{thumbErr}</div>}
            {thumbUrl && (
              <div style={{marginTop:10}}>
                <img src={thumbUrl} alt="thumbnail"
                  style={{width:'100%',borderRadius:'var(--r)',border:'1px solid var(--border)',display:'block'}} />
                <a className="btn btn-gold btn-full" href={thumbUrl} download="thumbnail.jpg"
                  style={{marginTop:8,textDecoration:'none',textAlign:'center'}}>Tải thumbnail ↓</a>
              </div>
            )}
          </>
        ) : (
          /* Template Valentine: tạo qua Excel (2 file) */
          <div>
            <div style={{color:'var(--muted)',fontSize:11,marginBottom:8}}>
              Template <b>Valentine</b> — tạo từ Excel: lưu file ảnh + nhãn (1 lần), rồi chọn file chữ (serif + script).
            </div>

            {/* Bước 1: file ảnh + nhãn — lưu 1 lần, dùng lại */}
            <div style={{color:'var(--muted)',fontSize:11,marginBottom:4}}>
              <b>File ảnh + nhãn</b> — cột A = nhãn, cột còn lại = ảnh (chèn ảnh vào ô). Lưu 1 lần, dùng lại nhiều lần.
            </div>
            <input ref={imageInputRef} type="file" accept=".xlsx" style={{display:'none'}}
              onChange={e => { const f = e.target.files?.[0]; if (f) onSaveImages?.(f); e.target.value='' }} />
            <button className="btn btn-outline btn-full" onClick={()=>imageInputRef.current?.click()} disabled={savingImages}>
              {savingImages ? <><span className="spin">⟳</span> Đang lưu...</> : '📁 Chọn & lưu file ảnh + nhãn'}
            </button>
            {excelSavedInfo
              ? <div style={{color:'var(--green,#3fb950)',fontSize:11,marginTop:5}}>
                  ✓ Đã lưu{excelSavedInfo.fromServer ? ' (từ lần trước)' : ''}: {excelSavedInfo.images} ảnh, {excelSavedInfo.labels} nhãn
                </div>
              : <div style={{color:'var(--muted)',fontSize:11,marginTop:5}}>Chưa có file ảnh — hãy lưu file ảnh trước khi tạo.</div>}

            {/* Bước 2: file chữ (serif + script) → tạo thumbnail */}
            <div style={{color:'var(--muted)',fontSize:11,margin:'12px 0 4px'}}>
              <b>File chữ</b> — cột A = dòng serif (LIST HOMESTAY), cột B = script (Valentine). Bốc ngẫu nhiên mỗi cột.
            </div>
            <input ref={textInputRef} type="file" accept=".xlsx" style={{display:'none'}}
              onChange={e => { const f = e.target.files?.[0]; if (f) { setLastTextFile(f); onExcelThumbnail?.(f, { badge: vBadge }) } e.target.value='' }} />
            <button className="btn btn-outline btn-full" onClick={()=>textInputRef.current?.click()} disabled={excelBusy || !excelSavedInfo}>
              {excelBusy ? <><span className="spin">⟳</span> Đang tạo ảnh...</> : '📄 Chọn file chữ & tạo thumbnail'}
            </button>

            {excelErr && <div style={{color:'var(--red)',fontSize:11,marginTop:6}}>{excelErr}</div>}
            {excelUrl && (
              <div style={{marginTop:10}}>
                <img src={excelUrl} alt="thumbnail-excel"
                  style={{width:'100%',borderRadius:'var(--r)',border:'1px solid var(--border)',display:'block'}} />

                {/* Sửa tay địa chỉ + tạo lại (dùng lại file chữ vừa chọn) */}
                <div style={{marginTop:10,padding:'10px 10px 12px',border:'1px solid var(--border)',borderRadius:'var(--r)'}}>
                  <label style={{display:'block',fontSize:11,color:'var(--muted)',marginBottom:2}}>Địa chỉ (badge)</label>
                  <input className="inp" value={vBadge} placeholder="(tự động: Quận + Tỉnh)"
                    onChange={e=>setVBadge(e.target.value)} style={{marginBottom:10}} />
                  <button className="btn btn-outline btn-full" disabled={excelBusy || !excelSavedInfo}
                    onClick={() => onExcelThumbnail?.(lastTextFile, { badge: vBadge })}>
                    {excelBusy ? <><span className="spin">⟳</span> Đang tạo lại...</> : '🔄 Tạo lại'}
                  </button>
                </div>

                <a className="btn btn-gold btn-full" href={excelUrl} download="thumbnail-excel.jpg"
                  style={{marginTop:8,textDecoration:'none',textAlign:'center'}}>
                  Tải thumbnail ↓
                </a>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function ProgressBox({ status, text, pct, done, err }) {
  return (
    <div className="progress-box">
      <div className="prog-head">
        {!done&&!err&&<div className="prog-dot"/>}
        <span>{status}</span>
      </div>
      <div className="prog-track">
        <div className="prog-fill" style={{width:pct+'%',background:err?'var(--red)':'var(--gold)'}}/>
      </div>
      <div className="prog-text">{text||'—'}</div>
      {err && (
        <div style={{
          marginTop:10, padding:'10px 12px', borderRadius:8,
          background:'rgba(220,60,60,0.10)', border:'1px solid var(--red)',
          color:'var(--red)', fontSize:12.5, lineHeight:1.55, whiteSpace:'pre-wrap',
        }}>
          {err}
        </div>
      )}
    </div>
  )
}

// ── HistoryPanel ─────────────────────────────────────────────────────────
export function HistoryPanel({ refresh }) {
  const [history, setHistory] = useState([])
  const [preview, setPreview] = useState(null)
  const [selected, setSelected] = useState(() => new Set())

  useEffect(() => {
    const load = () => getHistory().then(({data})=>setHistory(data.records||[])).catch(()=>{})
    load()
    const t = setInterval(load, 4000)
    return ()=>clearInterval(t)
  }, [refresh])

  useEffect(() => {
    if (!preview) return
    const onKey = (e) => { if (e.key === 'Escape') setPreview(null) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [preview])

  const toggleSel = useCallback((filename, ev) => {
    ev.stopPropagation()
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(filename)) next.delete(filename)
      else next.add(filename)
      return next
    })
  }, [])

  const selectAllVisible = useCallback(() => {
    const fnames = [...history].reverse().map(r => r.filename).filter(Boolean)
    setSelected(new Set(fnames))
  }, [history])

  const clearSel = useCallback(() => setSelected(new Set()), [])

  const downloadSelected = useCallback(() => {
    const list = [...history].reverse().filter(r => r.filename && selected.has(r.filename))
    list.forEach((r, i) => {
      setTimeout(() => downloadUrlToDisk(r.download_url, r.filename), i * 450)
    })
  }, [history, selected])

  const rows = [...history].reverse()

  return (
    <div className="render-panel" style={{paddingTop:0}}>
      <hr className="sep"/>
      <div style={{display:'flex',alignItems:'center',justifyContent:'space-between',flexWrap:'wrap',gap:8}}>
        <span className="plabel">Lịch sử render</span>
        <span style={{fontFamily:'var(--mono)',fontSize:10,color:'var(--muted)'}}>{history.length} video</span>
      </div>
      {history.length > 0 && (
        <div className="hist-toolbar">
          <button type="button" className="btn btn-outline btn-sm" onClick={selectAllVisible}>Chọn hết</button>
          <button type="button" className="btn btn-outline btn-sm" onClick={clearSel}>Bỏ chọn</button>
          <button type="button" className="btn btn-gold btn-sm" disabled={selected.size===0} onClick={downloadSelected}>
            ⬇ Tải đã chọn ({selected.size})
          </button>
        </div>
      )}
      <div className="hist-scroll">
        {history.length===0
          ?<div className="empty-state">Chưa có video nào</div>
          :rows.map((r)=>(
            <div
              key={r.filename || r.render_time}
              className="hist-item"
              role="button"
              tabIndex={0}
              onClick={()=>setPreview(r)}
              onKeyDown={(e)=>{ if(e.key==='Enter'||e.key===' ') { e.preventDefault(); setPreview(r) } }}
            >
              <input
                type="checkbox"
                className="hist-cb"
                checked={r.filename ? selected.has(r.filename) : false}
                onChange={(e)=>r.filename&&toggleSel(r.filename,e)}
                onClick={(e)=>e.stopPropagation()}
                title="Chọn để tải hàng loạt"
              />
              <div className="hist-icon">🎬</div>
              <div className="hist-info">
                <div className="hist-name">{r.listing_name||'Video'}</div>
                <div className="hist-meta">{r.render_time} · {r.file_size_mb?.toFixed(1)} MB · {r.photo_count} ảnh · bấm xem</div>
              </div>
              <a
                className="hist-dl"
                href={r.download_url}
                download={r.filename}
                title="Tải xuống"
                onClick={(e)=>e.stopPropagation()}
              >⬇️</a>
            </div>
          ))
        }
      </div>
      <button className="btn btn-green btn-full" onClick={exportExcel} disabled={history.length===0}>
        Xuất Excel
      </button>

      {preview && (
        <div className="video-modal-backdrop" onClick={()=>setPreview(null)}>
          <div className="video-modal" onClick={(e)=>e.stopPropagation()}>
            <div className="video-modal-head">
              <span className="video-modal-title">{preview.listing_name || 'Video'}</span>
              <button type="button" className="video-modal-x" onClick={()=>setPreview(null)} aria-label="Đóng">×</button>
            </div>
            <video
              key={preview.filename}
              className="video-modal-player"
              src={preview.download_url}
              controls
              playsInline
              autoPlay
              muted
            />
            <div className="video-modal-foot">
              <span className="hist-meta">{preview.render_time} · {preview.filename}</span>
              <button type="button" className="btn btn-gold btn-sm" onClick={()=>downloadUrlToDisk(preview.download_url, preview.filename)}>
                ⬇ Tải video này
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── ThumbnailHistoryPanel ─────────────────────────────────────────────────
// Grid of the collage thumbnails produced during batch render (one per listing).
export function ThumbnailHistoryPanel({ refresh }) {
  const [items, setItems] = useState([])
  const [zoom, setZoom]   = useState(null)

  useEffect(() => {
    const load = () => getThumbnailHistory().then(({data})=>setItems(data.records||[])).catch(()=>{})
    load()
    const t = setInterval(load, 4000)
    return ()=>clearInterval(t)
  }, [refresh])

  useEffect(() => {
    if (!zoom) return
    const onKey = (e) => { if (e.key === 'Escape') setZoom(null) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [zoom])

  if (items.length === 0) return null  // ẩn cho đến khi có thumbnail

  const rows = [...items].reverse()
  const downloadAll = () => {
    rows.forEach((r, i) => setTimeout(() => downloadUrlToDisk(r.url, r.filename), i * 350))
  }

  return (
    <div className="render-panel" style={{paddingTop:0}}>
      <hr className="sep"/>
      <div style={{display:'flex',alignItems:'center',justifyContent:'space-between',flexWrap:'wrap',gap:8}}>
        <span className="plabel">Lịch sử thumbnail</span>
        <span style={{fontFamily:'var(--mono)',fontSize:10,color:'var(--muted)'}}>{items.length} ảnh</span>
      </div>
      <div className="hist-toolbar">
        <button type="button" className="btn btn-gold btn-sm" onClick={downloadAll}>⬇ Tải tất cả ({rows.length})</button>
      </div>
      <div className="thumb-grid">
        {rows.map((r) => (
          <div
            key={r.filename}
            className="thumb-card"
            role="button"
            tabIndex={0}
            onClick={()=>setZoom(r)}
            onKeyDown={(e)=>{ if(e.key==='Enter'||e.key===' '){ e.preventDefault(); setZoom(r) } }}
          >
            <img src={r.url} alt={r.listing_name||'thumbnail'} loading="lazy" className="thumb-card-img"/>
            <div className="thumb-card-name">{r.listing_name || 'Thumbnail'}</div>
            <a
              className="thumb-card-dl"
              href={r.url}
              download={r.filename}
              title="Tải xuống"
              onClick={(e)=>e.stopPropagation()}
            >⬇️</a>
          </div>
        ))}
      </div>

      {zoom && (
        <div className="video-modal-backdrop" onClick={()=>setZoom(null)}>
          <div className="video-modal" onClick={(e)=>e.stopPropagation()}>
            <div className="video-modal-head">
              <span className="video-modal-title">{zoom.listing_name || 'Thumbnail'}</span>
              <button type="button" className="video-modal-x" onClick={()=>setZoom(null)} aria-label="Đóng">×</button>
            </div>
            <img src={zoom.url} alt={zoom.listing_name||'thumbnail'} style={{width:'100%',display:'block',borderRadius:'var(--r)'}}/>
            <div className="video-modal-foot">
              <span className="hist-meta">{zoom.render_time} · {zoom.filename}</span>
              <button type="button" className="btn btn-gold btn-sm" onClick={()=>downloadUrlToDisk(zoom.url, zoom.filename)}>
                ⬇ Tải ảnh này
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
