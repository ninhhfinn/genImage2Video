import { useState, useRef } from 'react'
import { uploadImages, parseListings, selectListing, fetchDayladau } from '../api/client'
import GeoSelect from './GeoSelect'

// Local-date helpers for the Dayladau date pickers (avoid UTC shift from toISOString).
const pad = n => String(n).padStart(2, '0')
const ymd = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
const TODAY = ymd(new Date())
const TOMORROW = (() => { const d = new Date(); d.setDate(d.getDate() + 1); return ymd(d) })()

export default function SourcePanel({ onReady, onMultiSelect, toast, photoLimit }) {
  const [tab, setTab]       = useState('dayladau')
  const [drag, setDrag]     = useState(false)
  const [apiUrl, setApiUrl] = useState('')
  const [apiJson, setApiJson] = useState('')
  const [checkin, setCheckin]   = useState(TODAY)
  const [checkout, setCheckout] = useState(TOMORROW)
  const [guests, setGuests]     = useState(2)
  const [geo, setGeo]           = useState(null) // { province_code, ward_code, keyword, label } | null
  const [listings, setListings] = useState([])
  const [checked, setChecked]   = useState(new Set())
  const [singleId, setSingleId] = useState(null)
  const [thumbs, setThumbs]     = useState([])
  const [count, setCount]       = useState(0)
  const [loadingL, setLoadingL] = useState(false)
  const [loadingS, setLoadingS] = useState(false)
  const [apiError, setApiError] = useState(null)
  // Photo preview modal
  const [previewListing, setPreviewListing] = useState(null)
  const fileRef = useRef()

  // ── Upload ──
  const handleFiles = async (files) => {
    const imgs = Array.from(files).filter(f => f.type.startsWith('image/'))
    if (!imgs.length) { toast('Không có ảnh hợp lệ', 'err'); return }
    setThumbs(imgs.map(f => URL.createObjectURL(f)))
    try {
      const { data } = await uploadImages(imgs)
      setCount(data.count)
      onReady([{ count: data.count, uploaded: true, name: 'Upload' }])
      toast(`Đã tải ${data.count} ảnh ✓`)
    } catch (e) { toast(e.message, 'err') }
  }

  // ── Load listings từ Dayladau (app dùng riêng cho dayladau) ──
  const loadDayladau = async () => {
    if (checkin > checkout) { toast('Ngày trả phòng phải sau ngày nhận', 'err'); return }
    setLoadingL(true)
    setApiError(null)
    try {
      const { data } = await fetchDayladau({
        checkin, checkout, guests: Number(guests) || 2, limit: 20, offset: 1,
        // Dayladau dùng scheme mã tỉnh/phường CŨ (vd Hà Nội = "11", ward "14859",
        // vẫn còn cấp district) — KHÁC hẳn mã vn-geo mới ("01"/"00013"). Gửi mã
        // vn-geo vào → API lọc ra 0 listing. Nên chỉ gửi TÊN đã chọn làm keyword
        // (lọc theo tên hoạt động tốt). Backend vẫn nhận được province_code/ward_code
        // nếu sau này có nguồn mã đúng của Dayladau.
        address: geo?.keyword || '',
      })
      if (data.error) {
        const errInfo = { title: data.error, details: data.details || 'Backend không trả details.', source: 'Dayladau API' }
        console.error('[dayladau] backend error', data)
        setApiError(errInfo)
        toast(data.error, 'err')
        return
      }
      setListings(data.listings || [])
      setChecked(new Set())
      toast(`Tìm thấy ${data.count} listing từ Dayladau`)
    } catch (e) {
      const status = e.response?.status
      const data = e.response?.data
      const errInfo = {
        title: status ? `Lỗi HTTP ${status}` : 'Không gọi được backend',
        details: data?.details || data?.error || e.message,
        source: 'Dayladau API',
      }
      console.error('[dayladau] request failed', e)
      setApiError(errInfo)
      toast(errInfo.title, 'err')
    }
    finally { setLoadingL(false) }
  }

  // ── Load listings từ URL ──
  const loadListings = async () => {
    const url = apiUrl.trim()
    const jsonText = apiJson.trim()
    if (!url && !jsonText) { toast('Nhập URL API hoặc dán JSON', 'err'); return }
    setLoadingL(true)
    setApiError(null)
    try {
      let body = {}
      if (url) {
        if (url.startsWith('{') || url.startsWith('[')) {
          try {
            body.json = JSON.parse(url)
          } catch {
            const errInfo = { title: 'JSON không hợp lệ', details: 'Nội dung trong ô URL giống JSON nhưng không parse được.' }
            setApiError(errInfo)
            toast(errInfo.title, 'err')
            return
          }
        } else {
          const normalizedUrl = normalizeApiUrl(url)
          if (!normalizedUrl) {
            const errInfo = {
              title: 'URL API không hợp lệ',
              details: `Bạn đang nhập "${url}". Đây giống ID/token chứ không phải URL API. Hãy nhập URL đầy đủ dạng https://... hoặc dán JSON response vào ô bên dưới.`,
              source: url,
            }
            setApiError(errInfo)
            toast(errInfo.title, 'err')
            return
          }
          body.url = normalizedUrl
        }
      } else {
        try {
          body.json = JSON.parse(jsonText)
        } catch {
          const errInfo = { title: 'JSON không hợp lệ', details: 'Nội dung dán vào không parse được bằng JSON.parse().' }
          setApiError(errInfo)
          toast(errInfo.title, 'err')
          return
        }
      }
      const { data } = await parseListings(body)
      if (data.error) {
        const errInfo = {
          title: data.error,
          details: data.details || 'Backend không trả details.',
          source: url || 'JSON dán trực tiếp',
        }
        console.error('[parse-listings] backend error', { request: body, response: data })
        setApiError(errInfo)
        toast(data.error, 'err')
        return
      }
      setListings(data.listings || [])
      setChecked(new Set())
      toast(`Tìm thấy ${data.count} listing`)
    } catch (e) {
      const status = e.response?.status
      const data = e.response?.data
      const errInfo = {
        title: status ? `Lỗi HTTP ${status}` : 'Không gọi được backend',
        details: data?.details || data?.error || e.message,
        source: url || 'JSON dán trực tiếp',
      }
      console.error('[parse-listings] request failed', e)
      setApiError(errInfo)
      toast(errInfo.title, 'err')
    }
    finally { setLoadingL(false) }
  }

  // ── Multi-select toggle ──
  const toggleCheck = (e, l) => {
    e.stopPropagation()
    setChecked(prev => {
      const next = new Set(prev)
      next.has(l.id) ? next.delete(l.id) : next.add(l.id)
      return next
    })
  }

  const selectAll = () => {
    checked.size === listings.length
      ? setChecked(new Set())
      : setChecked(new Set(listings.map(l => l.id)))
  }

  // ── Confirm selection ──
  const confirmSelection = async () => {
    const selected = listings.filter(l => checked.has(l.id))
    if (!selected.length) { toast('Chưa chọn listing nào', 'err'); return }
    if (selected.length > 1) {
      onMultiSelect(selected)
      toast(`${selected.length} listing vào hàng chờ`)
      return
    }
    // Single listing
    const l = selected[0]
    setLoadingS(true); setSingleId(l.id)
    try {
      const { data } = await selectListing({
        photo_urls: l.photo_urls, listing_id: l.id||'',
        listing_name: l.nickname||l.name||'', address: l.address||'',
        photo_limit: photoLimit || 0,
      })
      if (data.error) { toast(data.error, 'err'); return }
      setCount(data.count)
      setThumbs((l.thumb_urls||l.photo_urls||[]).slice(0, data.count || photoLimit || undefined))
      onReady([{ ...l, count: data.count }])
      toast(`${l.nickname||l.name} — ${data.count} ảnh sẵn sàng`)
    } catch (e) { toast(e.message, 'err') }
    finally { setLoadingS(false) }
  }

  const allChecked = listings.length > 0 && checked.size === listings.length

  return (
    <div>
      <div className="tabs">
        <button className={`tab${tab==='dayladau'?' on':''}`} onClick={()=>setTab('dayladau')}>Dayladau</button>
        <button className={`tab${tab==='upload'?' on':''}`} onClick={()=>setTab('upload')}>Tải lên</button>
        <button className={`tab${tab==='api'?' on':''}`} onClick={()=>setTab('api')}>URL API</button>
      </div>

      {/* ── Dayladau (mặc định) ── */}
      {tab==='dayladau' && (
        <div>
          <label style={{display:'flex',flexDirection:'column',gap:4,fontSize:12,fontWeight:600,marginBottom:8}}>
            Địa chỉ / khu vực
            <GeoSelect value={geo} onChange={setGeo} />
          </label>
          <div style={{display:'grid',gridTemplateColumns:'minmax(0,1fr) minmax(0,1fr) 64px',gap:8,alignItems:'end'}}>
            <label style={{minWidth:0,display:'flex',flexDirection:'column',gap:4,fontSize:12,fontWeight:600}}>
              Nhận phòng <span style={{color:'var(--muted,#999)',fontWeight:400}}>14:00</span>
              <input type="date" className="inp" style={{minWidth:0}} value={checkin} onChange={e=>setCheckin(e.target.value)}/>
            </label>
            <label style={{minWidth:0,display:'flex',flexDirection:'column',gap:4,fontSize:12,fontWeight:600}}>
              Trả phòng <span style={{color:'var(--muted,#999)',fontWeight:400}}>11:00</span>
              <input type="date" className="inp" style={{minWidth:0}} value={checkout} onChange={e=>setCheckout(e.target.value)}/>
            </label>
            <label style={{minWidth:0,display:'flex',flexDirection:'column',gap:4,fontSize:12,fontWeight:600}}>
              Khách
              <select className="inp" value={guests} onChange={e=>setGuests(e.target.value)}>
                {[1,2,3,4,5,6,8,10].map(n=><option key={n} value={n}>{n}</option>)}
              </select>
            </label>
          </div>
          <button
            className="btn btn-gold"
            style={{width:'100%',marginTop:12}}
            onClick={loadDayladau}
            disabled={loadingL}
          >
            {loadingL ? <><span className="spin">⟳</span> Đang tải…</> : 'Tải listing từ Dayladau'}
          </button>
        </div>
      )}

      {/* ── Upload ── */}
      {tab==='upload' && (
        <div
          className={`dropzone${drag?' drag':''}`}
          onDragOver={e=>{e.preventDefault();setDrag(true)}}
          onDragLeave={()=>setDrag(false)}
          onDrop={e=>{e.preventDefault();setDrag(false);handleFiles(e.dataTransfer.files)}}
        >
          <input ref={fileRef} type="file" multiple accept="image/*" onChange={e=>handleFiles(e.target.files)}/>
          <div className="dz-icon">🖼️</div>
          <div className="dz-main">Kéo thả ảnh vào đây</div>
          <div className="dz-sub">JPG · PNG · WEBP</div>
        </div>
      )}

      {/* ── URL API / JSON ── */}
      {tab==='api' && (
        <div>
          <div className="row center">
            <input
              className="inp"
              placeholder="https://api.example.com/listings..."
              value={apiUrl}
              onChange={e=>setApiUrl(e.target.value)}
              onKeyDown={e=>e.key==='Enter'&&loadListings()}
            />
            <button className="btn btn-gold btn-sm" onClick={loadListings} disabled={loadingL}>
              {loadingL ? <span className="spin">⟳</span> : 'Tải'}
            </button>
          </div>
          <div className="divider-text">hoặc</div>
          <textarea
            className="inp textarea"
            placeholder='Dán JSON API vào đây nếu URL không tải được'
            value={apiJson}
            onChange={e=>setApiJson(e.target.value)}
            style={{minHeight:90}}
          />
        </div>
      )}

      {/* ── Lỗi tải listing (Dayladau / URL API) ── */}
      {apiError && (
        <div className="api-error">
          <div className="api-error-title">{apiError.title}</div>
          {apiError.source && <div className="api-error-line">Nguồn: {apiError.source}</div>}
          <pre>{apiError.details}</pre>
        </div>
      )}

      {/* ── Listing picker ── */}
      {listings.length > 0 && (
        <div style={{marginTop:16}}>
          {/* Select bar */}
          <div className="select-bar">
            <div className="select-bar-left">
              <input
                type="checkbox" checked={allChecked} onChange={selectAll}
                style={{width:15,height:15,accentColor:'var(--gold)',cursor:'pointer'}}
              />
              <span style={{fontSize:12,fontWeight:600}}>
                {checked.size > 0 ? 'Đã chọn' : 'Chọn tất cả'}
              </span>
              {checked.size > 0 && (
                <span className="sel-count">{checked.size}/{listings.length}</span>
              )}
            </div>
            <div style={{display:'flex',gap:6}}>
              {checked.size > 0 && (
                <button className="btn btn-outline btn-sm" onClick={()=>setChecked(new Set())}>Bỏ chọn</button>
              )}
              <button
                className="btn btn-gold btn-sm"
                onClick={confirmSelection}
                disabled={checked.size===0||loadingS}
              >
                {loadingS ? <><span className="spin">⟳</span> Đang tải...</>
                  : checked.size === 0 ? 'Chọn listing'
                  : checked.size === 1 ? '✓ Xác nhận'
                  : `Render ${checked.size} listing`}
              </button>
            </div>
          </div>

          {/* Grid */}
          <div className="listing-scroll">
            <div className="listing-grid">
              {listings.map((l, i) => (
                <ListingCard
                  key={l.id||i} l={l}
                  checked={checked.has(l.id)}
                  active={singleId===l.id}
                  loading={loadingS&&singleId===l.id}
                  onToggle={e=>toggleCheck(e,l)}
                  onPreview={()=>setPreviewListing(l)}
                />
              ))}
            </div>
          </div>
        </div>
      )}

      {/* ── Thumbnail strip ── */}
      {thumbs.length > 0 && (
        <div style={{marginTop:14}}>
          <span className="plabel">Xem trước</span>
          <div className="thumb-grid">
            {thumbs.slice(0,9).map((url,i)=>(
              <div key={i} className="thumb">
                <img src={url} loading="lazy" alt=""/>
                <div className="thumb-n">{i+1}</div>
              </div>
            ))}
            {thumbs.length > 9 && <div className="thumb more-thumb">+{thumbs.length-9}</div>}
          </div>
          <div className="stats-bar">
            <div className="stat-box"><div className="stat-val">{count}</div><div className="stat-lbl">Ảnh</div></div>
            <div className="stat-box"><div className="stat-val" style={{color:'var(--green)'}}>✓</div><div className="stat-lbl">Sẵn sàng</div></div>
          </div>
        </div>
      )}

      {/* ── Photo Preview Modal ── */}
      {previewListing && (
        <PhotoModal
          listing={previewListing}
          onClose={()=>setPreviewListing(null)}
          onSelect={async () => {
            setPreviewListing(null)
            const l = previewListing
            // Toggle check
            setChecked(prev => {
              const next = new Set(prev)
              next.add(l.id)
              return next
            })
            // Nếu chỉ chọn listing này → confirm ngay
            setLoadingS(true); setSingleId(l.id)
            try {
              const { data } = await selectListing({
                photo_urls: l.photo_urls, listing_id: l.id||'',
                listing_name: l.nickname||l.name||'', address: l.address||'',
                photo_limit: photoLimit || 0,
              })
              if (data.error) { toast(data.error, 'err'); return }
              setCount(data.count)
              setThumbs((l.thumb_urls||l.photo_urls||[]).slice(0, data.count || photoLimit || undefined))
              onReady([{ ...l, count: data.count }])
              toast(`${l.nickname||l.name} — ${data.count} ảnh sẵn sàng`)
            } catch (e) { toast(e.message, 'err') }
            finally { setLoadingS(false) }
          }}
        />
      )}
    </div>
  )
}

function normalizeApiUrl(value) {
  const s = value.trim()
  if (/^https?:\/\//i.test(s)) return s
  if (/^[\w.-]+\.[a-z]{2,}(\/.*)?$/i.test(s)) return `https://${s}`
  return ''
}

// ── Listing Card ──────────────────────────────────────────────────────────
function ListingCard({ l, checked, active, loading, onToggle, onPreview }) {
  const thumb = l.thumb_urls?.[0]||''
  const name  = l.nickname||l.name||'Listing'
  const nPics = l.photo_count||l.photo_urls?.length||0
  return (
    <div
      className={`lcard${checked?' checked':''}${active?' sel':''}`}
      onClick={onPreview}  // click card → xem ảnh
    >
      <div className="lcard-img">
        {thumb ? <img src={thumb} loading="lazy" alt=""/> : <div style={{height:'100%',background:'var(--bg4)'}}/>}
        <div className="lcard-badge">🖼 {nPics}</div>
        {/* Checkbox góc phải trên */}
        <div
          className="lcard-check"
          onClick={onToggle}  // click checkbox → chọn/bỏ
          title={checked?'Bỏ chọn':'Chọn listing này'}
        >
          {checked?'✓':''}
        </div>
        {active && <div className="sel-badge">{loading?'⟳':'✓'}</div>}
      </div>
      <div className="lcard-body">
        <div className="lcard-name">{name}</div>
        <div className="lcard-addr">{l.address||'—'}</div>
        {l.price>0&&<div className="lcard-price">{l.price.toLocaleString('vi-VN')}₫/đêm</div>}
      </div>
    </div>
  )
}

// ── Photo Preview Modal ───────────────────────────────────────────────────
function PhotoModal({ listing, onClose, onSelect }) {
  const name   = listing.nickname||listing.name||'Listing'
  const photos = listing.photo_urls||[]
  const thumbs = listing.thumb_urls||photos
  const nPics  = photos.length

  // Click backdrop để đóng
  const handleBackdrop = (e) => { if (e.target===e.currentTarget) onClose() }

  return (
    <div className="modal-overlay" onClick={handleBackdrop}>
      <div className="modal">
        <div className="modal-head">
          <div>
            <div className="modal-title">{name}</div>
            <div className="modal-sub">{listing.address||''} · {nPics} ảnh</div>
          </div>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <div className="modal-body">
          <div className="modal-photos">
            {thumbs.map((url,i) => (
              <div key={i} className="modal-photo">
                <img src={url} loading="lazy" alt={`Ảnh ${i+1}`}/>
              </div>
            ))}
          </div>
        </div>

        <div className="modal-foot">
          <span className="modal-foot-info">
            {listing.price>0 && <>{listing.price.toLocaleString('vi-VN')}₫/đêm · </>}
            {nPics} ảnh
          </span>
          <div style={{display:'flex',gap:8}}>
            <button className="btn btn-outline btn-sm" onClick={onClose}>Đóng</button>
            <button className="btn btn-gold btn-sm" onClick={onSelect}>
              ✓ Chọn listing này
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
