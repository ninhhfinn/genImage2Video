import { useEffect, useMemo, useRef, useState } from 'react'

// Searchable địa chỉ / khu vực picker driven by vn-geo data
// (frontend/public/geo/{provinces,wards}.json — 2-cấp: tỉnh/thành + phường/xã).
// Search khớp THEO TÊN tỉnh và TÊN phường/xã (bỏ dấu). Khi chọn, phát ra
// { level, province_code, ward_code, keyword, label } cho caller gửi lên API.

// Bỏ dấu tiếng Việt để search không phân biệt dấu (gõ "ha noi" ra "Hà Nội").
const norm = (s) =>
  (s || '')
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/đ/g, 'd')
    .trim()

// Load + index 1 lần cho cả app (cache ở module scope, tránh fetch lại mỗi mount).
let _geoCache = null
let _geoPromise = null
function loadGeo() {
  if (_geoCache) return Promise.resolve(_geoCache)
  if (_geoPromise) return _geoPromise
  _geoPromise = Promise.all([
    fetch('/geo/provinces.json').then(r => r.json()),
    fetch('/geo/wards.json').then(r => r.json()),
  ]).then(([provinces, wards]) => {
    const byCode = {}
    const prov = provinces.map(p => {
      const o = { type: 'province', code: p.code, name: p.name, fullName: p.fullName || p.name, norm: norm(p.name) }
      byCode[p.code] = o
      return o
    })
    const ward = wards.map(w => ({
      type: 'ward', code: w.code, name: w.name, fullName: w.fullName || w.name,
      provinceCode: w.provinceCode, provinceName: byCode[w.provinceCode]?.name || '',
      norm: norm(w.name),
    }))
    _geoCache = { provinces: prov, wards: ward }
    return _geoCache
  })
  return _geoPromise
}

export default function GeoSelect({ value, onChange, placeholder = 'Tất cả khu vực' }) {
  const [geo, setGeo]     = useState(_geoCache)
  const [open, setOpen]   = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const rootRef  = useRef(null)
  const inputRef = useRef(null)
  const listRef  = useRef(null)

  // Nạp dữ liệu lần đầu khi mở dropdown.
  useEffect(() => {
    if (open && !geo) loadGeo().then(setGeo)
  }, [open, geo])

  // Focus ô search + reset khi mở.
  useEffect(() => {
    if (open) { setQuery(''); setActive(0); inputRef.current?.focus() }
  }, [open])

  // Click ra ngoài → đóng.
  useEffect(() => {
    if (!open) return
    const onDown = (e) => { if (rootRef.current && !rootRef.current.contains(e.target)) setOpen(false) }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  const results = useMemo(() => {
    if (!geo) return []
    const q = norm(query)
    if (!q) return geo.provinces // query rỗng → duyệt 34 tỉnh/thành
    const hits = []
    const collect = (item) => {
      const idx = item.norm.indexOf(q)
      if (idx !== -1) hits.push({ item, starts: idx === 0 })
    }
    geo.provinces.forEach(collect)
    geo.wards.forEach(collect)
    hits.sort((a, b) =>
      (b.starts - a.starts) ||                                    // prefix-match lên trước
      ((a.item.type === 'province' ? 0 : 1) - (b.item.type === 'province' ? 0 : 1)) || // tỉnh trước phường
      a.item.name.localeCompare(b.item.name, 'vi'))
    return hits.slice(0, 60).map(h => h.item)
  }, [geo, query])

  const commit = (item) => {
    const sel = item.type === 'province'
      ? { level: 'province', province_code: item.code, ward_code: '', keyword: item.name, label: item.fullName || item.name }
      : { level: 'ward', province_code: item.provinceCode, ward_code: item.code, keyword: item.name, label: item.fullName || item.name }
    onChange(sel)
    setOpen(false)
    setQuery('')
  }

  const clear = (e) => { e.stopPropagation(); onChange(null); setQuery('') }

  const onKeyDown = (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive(a => Math.min(a + 1, results.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive(a => Math.max(a - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); results[active] && commit(results[active]) }
    else if (e.key === 'Escape') { setOpen(false) }
  }

  // Cuộn item đang active vào tầm nhìn.
  useEffect(() => {
    if (!open) return
    listRef.current?.querySelector('.geo-opt.active')?.scrollIntoView({ block: 'nearest' })
  }, [active, open])

  const label = value?.label

  return (
    <div className="geo-select" ref={rootRef}>
      <button type="button" className={`inp geo-control${open ? ' open' : ''}`} onClick={() => setOpen(o => !o)}>
        <span className={`geo-value${label ? '' : ' ph'}`}>{label || placeholder}</span>
        {label && <span className="geo-clear" role="button" title="Xoá khu vực" onClick={clear}>✕</span>}
        <span className="geo-caret">▾</span>
      </button>

      {open && (
        <div className="geo-panel">
          <input
            ref={inputRef}
            className="geo-search"
            placeholder="Tìm tỉnh / phường, xã…"
            value={query}
            onChange={e => { setQuery(e.target.value); setActive(0) }}
            onKeyDown={onKeyDown}
          />
          <div className="geo-list" ref={listRef}>
            {!geo && <div className="geo-empty">Đang tải dữ liệu…</div>}
            {geo && results.length === 0 && <div className="geo-empty">Không tìm thấy khu vực</div>}
            {geo && results.map((item, i) => (
              <div
                key={`${item.type}-${item.code}`}
                className={`geo-opt${i === active ? ' active' : ''}`}
                onMouseEnter={() => setActive(i)}
                // Chọn ngay trên mousedown (trước blur/re-render) — click event có thể
                // bị bỏ lỡ nếu list re-render giữa mousedown và mouseup (rõ nhất với
                // phường/xã vì list đang lọc theo query), khiến dropdown không đóng.
                onMouseDown={e => { e.preventDefault(); commit(item) }}
              >
                <span className={`geo-badge ${item.type}`}>{item.type === 'province' ? 'Tỉnh/TP' : 'P/X'}</span>
                <span className="geo-opt-main">
                  <span className="geo-opt-name">{item.name}</span>
                  {item.type === 'ward' && <span className="geo-opt-sub">{item.fullName}</span>}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
