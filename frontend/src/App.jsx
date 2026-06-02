import './index.css'
import { useState, useCallback, useRef } from 'react'
import { useToast }   from './hooks/useToast'
import { useRender }  from './hooks/useRender'
import SourcePanel    from './components/SourcePanel'
import SettingsPanel  from './components/SettingsPanel'
import { RenderPanel, HistoryPanel, ThumbnailHistoryPanel } from './components/RenderPanel'
import { selectListing, renderThumbnail } from './api/client'

const DEFAULT_SETTINGS = {
  mode: 'kenburns',
  duration: 3,
  photoLimit: 12,
  zoom: 0.3,
  tiktok: true,
  title: '',
  titleDuration: 3,
  watermark: '',
  effectType: 'move-left-right',
  useOverlay: true,
  overlayPriceTypes: ['two_day_one_night', 'overnight', 'hourly', 'extra_hour', 'hour_combo'],
  overlayStyle: 'badge',
  overlayFont: '',
  overlayScale: 1,
  overlayText: '#FFFFFF',
  overlayBg: '',
  overlayStroke: '#000000',
  titleColor: '#FFFFFF',
  strokeColor: '#b3471f',
  titleBg: '',
  bodyBg: '',
  textFont: '',
  textScale: 1,
  textColor: '#FFFFFF',
  amenitiesText: '',
  template: 'daiky',
  gridIntro: true,
  batchMode: 'both',      // mẻ nhiều listing xuất gì: 'video' | 'thumbnail' | 'both'
  customFont: '',         // font tự tải lên cho tiêu đề (video + thumbnail); '' = theo template

  // ── Thumbnail (ảnh collage tĩnh) — để trống, tự chỉnh sau ──
  thumbTemplate: '',      // '' classic (hero+polaroid) | daiky | valey | peony | tiger
  thumbTitle: '',         // chữ tiêu đề lớn (vd "Sunset"); trống → lấy nickname
  thumbPrice: '',         // dòng giá gọn (vd "2h 249k- 4h 367k- Qua đêm 449k"); trống → tự ghép từ listing
  thumbWatermark: '',     // @handle
  thumbTitleFont: 'Baloo2-Bold.ttf',  // Baloo tròn | Baloo2-ExtraBold.ttf | Prata-Regular.ttf
  thumbStrokeColor: '#b3471f',
  thumbStrokeWidth: 6,
  thumbPillColor: '#000000',
  thumbPillAlpha: 0.32,
  thumbTitleScale: 1,     // cỡ chữ tiêu đề (×)
  thumbDataScale: 1,      // cỡ chữ data (×)
}

export default function App() {
  const [toasts, toast]           = useToast()
  const [uploadedCount, setCount] = useState(0)
  const [listingName, setName]    = useState('')
  const [activeListing, setActL]  = useState(null)
  const [settings, setSettings]   = useState(DEFAULT_SETTINGS)
  const [histRefresh, setRefresh] = useState(0)
  const [thumbRefresh, setThumbRefresh] = useState(0)

  // Thumbnail state
  const [thumbUrl, setThumbUrl]   = useState('')
  const [thumbBusy, setThumbBusy] = useState(false)
  const [thumbErr, setThumbErr]   = useState('')

  const [queue, setQueue]   = useState([])
  const queueIdxRef = useRef(-1)
  const queueRef = useRef([])

  const { render, rendering, done, pct, progText, status, error } = useRender(
    () => {
      setRefresh(n => n + 1)
      runNextInQueue()
    },
    () => failCurrentInQueue(),  // render lỗi → đánh ✗ rồi nhảy listing kế, không đứng queue
  )

  // ── Build cfg gửi API render ──
  const buildRenderCfg = useCallback((listing) => {
    const {
      mode, duration, zoom, tiktok, title, titleDuration, watermark, effectType, useOverlay,
      overlayPriceTypes, overlayStyle, overlayFont, overlayScale, overlayText, overlayBg, overlayStroke,
      titleColor, strokeColor, titleBg, bodyBg, textFont, textScale, textColor, amenitiesText, template,
      gridIntro,
    } = settings
    let kind = effectType || 'move-left-right'
    if (kind !== 'random') {
      if (kind === 'pan-right') kind = 'move-left-right'
      if (kind === 'pan-left') kind = 'move-right-left'
    }
    const safeDuration = Math.max(1.5, Number(duration) || 3)
    const cfg = {
      mode, duration: safeDuration, total: safeDuration, zoom_intensity: zoom, fps: 30, tiktok,
      width:  tiktok ? 1080 : 1920,
      height: tiktok ? 1920 : 1080,
      title, title_duration: Math.max(0.5, Number(titleDuration) || 3), watermark,
      text_font: textFont || '',
      text_scale: Number(textScale) || 1,
      text_color: textColor || '#FFFFFF',
      effect_type: kind,
      effect_types: [kind],
      template: template || 'daiky',
      grid_intro: !!gridIntro,
      title_font_file: settings.customFont || '',
    }
    // Overlay từ listing
    if (useOverlay && listing) {
      const selectedPriceTypes = Array.isArray(overlayPriceTypes)
        ? new Set(overlayPriceTypes)
        : null
      const priceLines = listing.price_line_items?.length
        ? listing.price_line_items
            .filter(item => !selectedPriceTypes || selectedPriceTypes.has(item.key))
            .map(item => item.text)
        : listing.price_lines || []
      cfg.address       = listing.address    || ''
      cfg.nickname      = listing.nickname   || listing.name || ''
      cfg.listing_id    = listing.id         || ''
      cfg.prices        = priceLines
      cfg.overlay_style = overlayStyle === 'bubble' ? 'bubble' : 'badge'
      cfg.overlay_font = overlayFont || ''
      cfg.overlay_scale = Number(overlayScale) || 1
      cfg.overlay_text = overlayText || '#FFFFFF'
      cfg.overlay_bg = overlayBg || ''
      cfg.overlay_stroke = overlayStroke || '#000000'
      cfg.title_color = titleColor || '#FFFFFF'
      cfg.stroke_color = strokeColor || '#b3471f'
      cfg.title_bg = titleBg || ''
      cfg.body_bg = bodyBg || ''
      const manualAmenities = (amenitiesText || '')
        .split(/[\n;,]+/)
        .flatMap(s => s.split(/\s*-\s+/))
        .map(s => s.trim())
        .filter(Boolean)
      cfg.amenities = manualAmenities.length ? manualAmenities : (listing.amenities || [])
    }
    return cfg
  }, [settings])

  // ── Build cfg gửi API render-thumbnail ──
  const buildThumbnailCfg = useCallback((listing) => {
    const s = settings
    // Random: mỗi listing chọn ngẫu nhiên 1 trong 4 template lưới.
    const GRID_TEMPLATES = ['daiky', 'valey', 'peony', 'tiger']
    let resolvedTemplate = s.thumbTemplate || ''
    if (resolvedTemplate === 'random') {
      resolvedTemplate = GRID_TEMPLATES[Math.floor(Math.random() * GRID_TEMPLATES.length)]
    }
    const firstWord = (str) => (str || '').trim().split(/\s+/)[0] || ''
    const title = (s.thumbTitle || '').trim()
      || listing?.nickname
      || firstWord(listing?.name)
      || ''
    // price line: dùng ô nhập tay nếu có; không thì ghép GỌN từ listing
    // (text gốc dài "Giá 2 ngày 1 đêm: Từ 414.300 đ" → "2N1Đ 414k").
    let prices
    if ((s.thumbPrice || '').trim()) {
      prices = s.thumbPrice.split(/\s*-\s+|\n/).map(x => x.trim()).filter(Boolean)
    } else {
      const KEY_LABEL = {
        two_day_one_night: '2N1Đ', overnight: 'Qua đêm',
        hourly: 'Giờ đầu', extra_hour: 'Thêm giờ', hour_combo: 'Combo',
      }
      const compactAmount = (text) => {
        // "đ(?!\p{L})": chỉ khớp "đ" là đơn vị tiền (không theo sau bởi chữ cái),
        // tránh khớp nhầm "1 đ" trong "1 đêm" → bug "2N1Đ 1".
        const m = (text || '').match(/([\d][\d.]*)\s*đ(?!\p{L})/u) || (text || '').match(/([\d][\d.]*)/)
        if (!m) return ''
        const n = parseInt(m[1].replace(/\./g, ''), 10)
        if (!n) return ''
        return n >= 1000 ? Math.round(n / 1000) + 'k' : String(n)
      }
      const sel = new Set(s.overlayPriceTypes || [])
      const items = (listing?.price_line_items || []).filter(it => !sel.size || sel.has(it.key))
      const labelFor = (it) => {
        if (it.key === 'hourly') {
          const mh = (it.text || '').match(/(\d+)\s*giờ đầu/) // "Giá 4 giờ đầu" → "4h đầu"
          if (mh) return `${mh[1]}h đầu`
        }
        return KEY_LABEL[it.key] || ''
      }
      prices = items.length
        ? items.map(it => [labelFor(it), compactAmount(it.text)].filter(Boolean).join(' ')).filter(Boolean)
        : (listing?.price_lines || [])
    }
    // amenities: dùng chung ô override với video
    const manual = (s.amenitiesText || '')
      .split(/[\n;,]+/)
      .flatMap(x => x.split(/\s*-\s+/))
      .map(x => x.trim())
      .filter(Boolean)
    const amenities = manual.length ? manual : (listing?.amenities || [])

    // Bảng giá cho template Valey (Trong tuần / Cuối tuần) từ prices_by_week.
    let price_table = []
    if (resolvedTemplate === 'valey') {
      const k = (n) => (n >= 1000 ? Math.round(n / 1000) + 'k' : String(n || ''))
      const pbw = listing?.prices_by_week || []
      const wd = pbw[1] || pbw[0] || 0          // ngày thường (thứ 2)
      const we = pbw[6] || pbw[5] || wd          // cuối tuần (thứ 7)
      const pfh = listing?.price_first_hours || 0
      const fh = listing?.first_hours || 0
      const nsr = listing?.night_short_rate || 0.3
      if (pfh) price_table.push({ slot: (fh ? fh + 'h' : 'Giờ') + ' đầu', weekday: k(pfh), weekend: k(pfh) })
      if (wd) price_table.push({ slot: '2 ngày 1 đêm', weekday: k(wd), weekend: k(we) })
      if (wd) price_table.push({ slot: 'Qua đêm', weekday: k(Math.round((nsr || 0.3) * wd)), weekend: k(Math.round((nsr || 0.3) * we)) })
    }

    return {
      title,
      address: listing?.address || '',
      prices,
      amenities,
      watermark: (s.thumbWatermark || '').trim(),
      width: 960,
      height: 1280,
      title_font: s.customFont || s.thumbTitleFont || 'Baloo2-Bold.ttf',
      title_color: s.titleColor || '#FFFFFF',
      stroke_color: s.thumbStrokeColor || '#b3471f',
      stroke_width: Number(s.thumbStrokeWidth) || 6,
      pill_color: s.thumbPillColor || '#000000',
      pill_alpha: Number(s.thumbPillAlpha) || 0.32,
      title_scale: Number(s.thumbTitleScale) || 1,
      data_scale: Number(s.thumbDataScale) || 1,
      thumb_template: resolvedTemplate,
      listing_id: listing?.id || '',
      price_table,
    }
  }, [settings])

  const generateThumbnail = useCallback(async () => {
    setThumbBusy(true); setThumbErr('')
    try {
      const { data } = await renderThumbnail(buildThumbnailCfg(activeListing))
      if (data.error) throw new Error(data.error)
      setThumbUrl(data.url)
      toast('Đã tạo thumbnail ✓', 'ok')
    } catch (e) {
      setThumbErr(e?.response?.data?.error || e.message)
      toast('Lỗi tạo thumbnail', 'err')
    } finally {
      setThumbBusy(false)
    }
  }, [buildThumbnailCfg, activeListing, toast])

  const handleReady = (items) => {
    const first = items[0]
    setCount(first.count || 0)
    setName(first.name || first.nickname || '')
    setActL(first.uploaded ? null : first)
    setQueue([])
    queueRef.current = []
    queueIdxRef.current = -1
  }

  const handleMultiSelect = useCallback((listings) => {
    const q = listings.map(l => ({ listing: l, status: 'pending' }))
    setQueue(q)
    queueRef.current = q
    queueIdxRef.current = -1
    setCount(0)
    setName(`${listings.length} listings`)
    toast(`${listings.length} listing đã vào hàng chờ, tự động render`)
    setTimeout(() => startQueueItem(0), 300)
  }, [toast, settings])

  const handleRender = async () => {
    if (queue.length > 0) {
      const idx = queue.findIndex(q => q.status === 'pending')
      if (idx === -1) { toast('Tất cả đã render xong', 'ok'); return }
      await startQueueItem(idx)
    } else {
      render(buildRenderCfg(activeListing))
    }
  }

  const startQueueItem = async (idx) => {
    const item = queueRef.current[idx]
    if (!item) return
    queueIdxRef.current = idx
    setQueue(q => {
      const next = q.map((x,i) => i===idx ? {...x, status:'running'} : x)
      queueRef.current = next
      return next
    })
    setName(item.listing.nickname || item.listing.name || '')

    try {
      const { data } = await selectListing({
        photo_urls:   item.listing.photo_urls,
        listing_id:   item.listing.id || '',
        listing_name: item.listing.nickname || item.listing.name || '',
        address:      item.listing.address || '',
        photo_limit:  settings.photoLimit || 0,
      })
      if (data.error) throw new Error(data.error)
      setCount(data.count)
    } catch(e) {
      setQueue(q => {
        const next = q.map((x,i) => i===idx ? {...x, status:'err'} : x)
        queueRef.current = next
        const nextIdx = next.findIndex(x => x.status === 'pending')
        if (nextIdx >= 0) setTimeout(() => startQueueItem(nextIdx), 800)
        return next
      })
      toast(`Lỗi listing ${idx+1}: ${e.message}`, 'err')
      return
    }
    // Mẻ nhiều listing: xuất theo lựa chọn (video | thumbnail | cả hai).
    const mode = settings.batchMode || 'both'
    if (mode !== 'video') {
      try {
        await renderThumbnail({
          ...buildThumbnailCfg(item.listing),
          save_history: true,
          listing_name: item.listing.nickname || item.listing.name || '',
          listing_id: item.listing.id || '',
        })
        setThumbRefresh(n => n + 1)
      } catch (_) { /* lỗi thumbnail không làm hỏng listing */ }
    }
    if (mode !== 'thumbnail') {
      render(buildRenderCfg(item.listing)) // video tự đẩy queue sang listing kế qua callback
    } else {
      runNextInQueue() // chỉ thumbnail → đánh dấu xong + tự sang listing kế
    }
  }

  const runNextInQueue = useCallback(() => {
    setQueue(prev => {
      const i = queueIdxRef.current
      if (i < 0) return prev
      const updated = prev.map((x,j) => j===i ? {...x, status:'done'} : x)
      queueRef.current = updated
      const nextIdx = updated.findIndex(x => x.status === 'pending')
      if (nextIdx >= 0) setTimeout(() => startQueueItem(nextIdx), 800)
      return updated
    })
  }, [settings])

  // Render lỗi (ffmpeg) giữa batch: đánh ✗ item đang chạy rồi nhảy listing kế,
  // để một video lỗi không làm đứng cả hàng chờ. Guard 'running' tránh phá hỏng
  // một batch đã xong khi sau đó render lẻ bị lỗi.
  const failCurrentInQueue = useCallback(() => {
    setQueue(prev => {
      const i = queueIdxRef.current
      if (i < 0 || !prev[i] || prev[i].status !== 'running') return prev
      const updated = prev.map((x,j) => j===i ? {...x, status:'err'} : x)
      queueRef.current = updated
      const nextIdx = updated.findIndex(x => x.status === 'pending')
      if (nextIdx >= 0) setTimeout(() => startQueueItem(nextIdx), 800)
      return updated
    })
  }, [settings])

  const isQueueMode  = queue.length > 0
  const pendingCount = queue.filter(q => q.status==='pending').length
  const doneCount    = queue.filter(q => q.status==='done').length

  return (
    <div className="app">
      <div className="topbar">
        <div className="logo">
          <div className="logo-box">🎬</div>
          <div className="logo-text">img<em>2</em>video</div>
        </div>
        <div className="topbar-pills">
          {uploadedCount>0&&<div className="pill active">📸 {uploadedCount} ảnh{listingName?` — ${listingName}`:''}</div>}
          {isQueueMode&&<div className="pill active">⏳ {doneCount}/{queue.length}</div>}
          <div className="pill">TikTok · Reels · Shorts</div>
        </div>
      </div>

      <div className="workspace">
        <div className="panel">
          <div className="phead"><span className="plabel">1 — Nguồn ảnh</span></div>
          <div className="pbody">
            <SourcePanel onReady={handleReady} onMultiSelect={handleMultiSelect} toast={toast} photoLimit={settings.photoLimit}/>
          </div>
        </div>

        <div className="panel">
          <div className="phead"><span className="plabel">2 — Cài đặt</span></div>
          <div className="pbody">
            <SettingsPanel settings={settings} onChange={setSettings} uploadedCount={uploadedCount} apiAmenities={activeListing?.amenities || []}/>
          </div>
        </div>

        <div className="panel">
          <div className="phead"><span className="plabel">3 — Xuất video</span></div>
          <RenderPanel
            canRender={uploadedCount>0 || pendingCount>0}
            onRender={handleRender}
            rendering={rendering}
            done={done && !isQueueMode}
            pct={pct} progText={progText} status={status} error={error}
            queue={queue} isQueueMode={isQueueMode}
            canThumbnail={uploadedCount>0}
            onThumbnail={generateThumbnail}
            thumbBusy={thumbBusy}
            thumbUrl={thumbUrl}
            thumbErr={thumbErr}
          />
          <HistoryPanel refresh={histRefresh}/>
          <ThumbnailHistoryPanel refresh={thumbRefresh}/>
        </div>
      </div>

      <div className="toast-wrap">
        {toasts.map(t=><div key={t.id} className={`toast ${t.type}`}>{t.msg}</div>)}
      </div>
    </div>
  )
}
