import TemplatePicker from './TemplatePicker'
import FontUpload from './FontUpload'

export default function SettingsPanel({ settings, onChange, uploadedCount, apiAmenities = [] }) {
  const {
    mode, duration, photoLimit, zoom, tiktok, title, titleDuration, watermark, effectType, useOverlay,
    overlayFont = 'playfair', overlayScale = 1, overlayText = '#FFFFFF',
    titleColor = '#FFFFFF', strokeColor = '#b3471f', titleBg = '', bodyBg = '',
  } = settings
  const overlayPriceTypes = settings.overlayPriceTypes || ['two_day_one_night', 'overnight', 'hourly', 'extra_hour', 'hour_combo']
  const template = settings.template || 'daiky'
  const gridIntro = settings.gridIntro !== false
  const amenitiesText = settings.amenitiesText || ''
  // Thumbnail controls
  const thumbTitle = settings.thumbTitle || ''
  const thumbTitleFont = settings.thumbTitleFont || 'Baloo2-Bold.ttf'
  const thumbPrice = settings.thumbPrice || ''
  const thumbWatermark = settings.thumbWatermark || ''
  const thumbStrokeColor = settings.thumbStrokeColor || '#b3471f'
  const thumbStrokeWidth = settings.thumbStrokeWidth ?? 6
  const thumbPillColor = settings.thumbPillColor || '#000000'
  const thumbPillAlpha = settings.thumbPillAlpha ?? 0.32
  const thumbTitleScale = settings.thumbTitleScale ?? 1
  const thumbDataScale = settings.thumbDataScale ?? 1
  // Tự đăng social
  const autoPost = settings.autoPost || false
  const webhookUrl = settings.webhookUrl || ''
  const postTiktok = settings.postTiktok !== false
  const postFacebook = settings.postFacebook !== false
  const perImg = Number(duration) || 3
  const titleSecs = Math.max(0.5, Number(titleDuration) || 3)
  const safePhotoLimit = Number(photoLimit) || 12
  const totalTime = uploadedCount > 0 ? (perImg * uploadedCount).toFixed(1) : '—'
  const set = (key) => (val) => onChange({ ...settings, [key]: val })
  const effectOptions = [
    ['zoom-in',  '🔍 Zoom In'],
    ['zoom-out', '🔎 Zoom Out'],
    ['move-left-right', 'Move Left → Right'],
    ['move-right-left', 'Move Right → Left'],
  ]
  const normalizeEffect = (value) => {
    if (value === 'pan-right') return 'move-left-right'
    if (value === 'pan-left') return 'move-right-left'
    return value // gồm 'random'
  }
  const legacyTypes = settings.effectTypes?.length ? settings.effectTypes : null
  const baseKind = effectType || legacyTypes?.[0] || 'move-left-right'
  const currentEffect = normalizeEffect(baseKind)
  const pickEffect = (value) => {
    const v = normalizeEffect(value)
    onChange({ ...settings, effectType: v })
  }
  const priceTypeOptions = [
    ['two_day_one_night', 'Giá 2 ngày 1 đêm', 'prices_by_week[2]'],
    ['overnight', 'Qua đêm', 'night_short_rate × prices_by_week thứ 2'],
    ['hourly', 'Giá giờ', 'price_first_hours'],
    ['extra_hour', 'Giá thêm giờ', 'price_per_hour'],
    ['hour_combo', 'Combo giờ', 'special_offer_times thứ 2'],
  ]
  const fontOptions = [
    ['playfair', 'Playfair Display', 'Serif sang, hợp badge'],
    ['lilita', 'Lilita One', 'Chữ dày, hợp bubble'],
    ['yeseva', 'Yeseva One', 'Serif cong, hợp Daiky'],
  ]
  const togglePriceType = (key) => {
    const next = new Set(overlayPriceTypes)
    next.has(key) ? next.delete(key) : next.add(key)
    onChange({ ...settings, overlayPriceTypes: [...next] })
  }

  return (
    <div>
      {/* Template picker — đầu danh sách */}
      <TemplatePicker
        value={settings.template || 'daiky'}
        onChange={(v) => set('template')(v)}
      />

      {/* Mode */}
      <div className="field">
        <label className="flabel">Hiệu ứng</label>
        <div className="seg">
          {[['kenburns','Ken Burns'],['slideshow','Slideshow'],['timelapse','Timelapse']].map(([v,l])=>(
            <button key={v} className={`seg-btn${mode===v?' on':''}`} onClick={()=>set('mode')(v)}>{l}</button>
          ))}
        </div>
      </div>

      {/* Effect type — chỉ khi Ken Burns */}
      {mode==='kenburns' && (
        <div className="field">
          <label className="flabel">Kiểu chuyển động Ken Burns</label>
          <div className="seg" style={{flexWrap:'wrap'}}>
            {effectOptions.map(([v,l])=>(
              <button key={v}
                className={`seg-btn${currentEffect===v?' on':''}`}
                style={{flex:'1 1 45%',minWidth:'45%'}}
                onClick={()=>pickEffect(v)}>
                {l}
              </button>
            ))}
            <button
              type="button"
              key="random"
              className={`seg-btn${currentEffect==='random'?' on':''}`}
              style={{flex:'1 1 100%',minWidth:'100%',marginTop:4}}
              onClick={()=>onChange({ ...settings, effectType: 'random' })}>
              🎲 Random — mỗi ảnh chọn ngẫu nhiên 1 trong 4 kiểu (một ảnh = một hiệu ứng)
            </button>
          </div>
          <div style={{fontSize:10,color:'var(--muted)',marginTop:6,fontStyle:'italic'}}>
            {currentEffect==='random'
              ? 'Mỗi cảnh (mỗi ảnh) được gán riêng zoom-in / zoom-out / pan trái→phải / pan phải→trái; không trộn nhiều kiểu trên cùng một ảnh.'
              : 'Chọn cố định: mọi ảnh dùng cùng họ hiệu ứng (có thể khác biến thể nhẹ giữa các ảnh). Pan = lia ngang.'}
          </div>
        </div>
      )}

      {/* Photo limit */}
      <div className="field">
        <label className="flabel">
          Số ảnh mỗi listing &nbsp;
          <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{safePhotoLimit}</span>
        </label>
        <div className="slider-row">
          <input type="range" min="1" max="40" step="1" value={safePhotoLimit} onChange={e=>set('photoLimit')(+e.target.value)}/>
          <span className="slider-val">{safePhotoLimit}</span>
        </div>
      </div>

      {/* Per-image time */}
      <div className="field">
        <label className="flabel">
          Thời gian từng ảnh &nbsp;
          <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{perImg}s</span>
          {uploadedCount>0&&<span style={{color:'var(--muted)',marginLeft:6}}>≈ tổng {totalTime}s</span>}
        </label>
        <div className="slider-row">
          <input type="range" min="1.5" max="10" step="0.5" value={perImg} onChange={e=>set('duration')(+e.target.value)}/>
          <span className="slider-val">{perImg}s</span>
        </div>
      </div>

      {/* Zoom intensity */}
      {mode==='kenburns' && (
        <div className="field">
          <label className="flabel">
            Cường độ zoom &nbsp;
            <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{zoom}</span>
          </label>
          <div className="slider-row">
            <input type="range" min="0" max="1" step="0.05" value={zoom} onChange={e=>set('zoom')(+e.target.value)}/>
            <span className="slider-val">{zoom}</span>
          </div>
        </div>
      )}

      {/* Format */}
      <div className="field">
        <label className="flabel">Định dạng</label>
        <div>
          <div className="toggle-row">
            <div className="ti">
              <div className="t1">9:16 TikTok / Reels</div>
              <div className="t2">1080 × 1920px</div>
            </div>
            <div className={`toggle${tiktok?' on':''}`} onClick={()=>set('tiktok')(!tiktok)}/>
          </div>
        </div>
      </div>

      {/* Grid intro toggle */}
      <div className="field">
        <label className="flabel">Ảnh bìa lưới</label>
        <div className="toggle-row">
          <div className="ti">
            <div className="t1">Lưới 2×2 ở đầu video</div>
            <div className="t2">Ghép 4 ảnh đầu (tĩnh) rồi về chuyển động 1 ảnh</div>
          </div>
          <div className={`toggle${gridIntro?' on':''}`} onClick={()=>set('gridIntro')(!gridIntro)}/>
        </div>
      </div>

      {/* Listing overlay toggle */}
      <div className="field">
        <label className="flabel">Hiển thị thông tin listing</label>
        <div className="toggle-row">
          <div className="ti">
            <div className="t1">Overlay xuyên suốt video</div>
            <div className="t2">Địa chỉ · Tên · Giá · ID</div>
          </div>
          <div className={`toggle${useOverlay?' on':''}`} onClick={()=>set('useOverlay')(!useOverlay)}/>
        </div>
        {useOverlay && (
          <div style={{marginTop:8}}>
            {priceTypeOptions.map(([key, label, source]) => (
              <div key={key} className="toggle-row">
                <div className="ti">
                  <div className="t1">{label}</div>
                  <div className="t2">{source}</div>
                </div>
                <div
                  className={`toggle${overlayPriceTypes.includes(key)?' on':''}`}
                  onClick={()=>togglePriceType(key)}
                />
              </div>
            ))}
            <div className="font-tool">
              <div className="flabel" style={{marginBottom:8}}>Tool chỉnh phông chữ listing</div>
              <div className="seg">
                {fontOptions.map(([v,l])=>(
                  <button
                    key={v}
                    className={`seg-btn${overlayFont===v?' on':''}`}
                    onClick={()=>set('overlayFont')(v)}
                  >
                    {l}
                  </button>
                ))}
              </div>
              <div className="font-preview" style={{fontFamily: overlayFont === 'lilita' ? '"Bebas Neue", sans-serif' : 'Georgia, serif'}}>
                {activePreviewText(settings)}
              </div>
              <div className="slider-row" style={{marginTop:8}}>
                <input type="range" min="0.5" max="1.35" step="0.05" value={overlayScale} onChange={e=>set('overlayScale')(+e.target.value)}/>
                <span className="slider-val">{Number(overlayScale).toFixed(2)}×</span>
              </div>
              <div className="color-grid">
                <label>
                  <span>Màu tiêu đề</span>
                  <input type="color" value={titleColor} onChange={e=>set('titleColor')(e.target.value)}/>
                </label>
                <label>
                  <span>Màu nội dung</span>
                  <input type="color" value={overlayText} onChange={e=>set('overlayText')(e.target.value)}/>
                </label>
                {template === 'sunset' && (
                  <label>
                    <span>Màu viền</span>
                    <input type="color" value={strokeColor} onChange={e=>set('strokeColor')(e.target.value)}/>
                  </label>
                )}
                {template !== 'sunset' && (
                  <label>
                    <span>Nền tiêu đề</span>
                    <input type="color" value={titleBg || '#6F4A30'} onChange={e=>set('titleBg')(e.target.value)}/>
                  </label>
                )}
                <label>
                  <span>Nền nội dung</span>
                  <input type="color" value={bodyBg || (template === 'daiky' ? '#A88B6E' : '#000000')} onChange={e=>set('bodyBg')(e.target.value)}/>
                </label>
              </div>
              <div style={{fontSize:10,color:'var(--muted)',marginTop:4,fontStyle:'italic'}}>
                Tách riêng màu chữ &amp; màu nền cho tiêu đề và nội dung. Viền chỉ áp cho Sunset.
              </div>
            </div>
            <div style={{marginTop:10}}>
              <div className="flabel" style={{marginBottom:4}}>Tiện nghi (mỗi dòng 1 mục, hoặc cách bằng " - ")</div>
              <textarea
                className="inp body-font textarea"
                placeholder={`Máy chiếu netflix\nBếp nấu\nCửa sổ thoáng\nGương checkin\nWc khép kín\nGửi xe free`}
                value={amenitiesText}
                onChange={e=>set('amenitiesText')(e.target.value)}
                style={{minHeight:80}}
              />
              <div style={{fontSize:10,color:'var(--muted)',marginTop:4,fontStyle:'italic'}}>
                Ghi đè amenities từ API. Để trống = dùng giá trị API (nếu có).
              </div>
              {apiAmenities.length > 0 && (
                <div style={{fontSize:10,color:'var(--gold)',marginTop:4}}>
                  ✓ API có {apiAmenities.length} tiện nghi: {apiAmenities.slice(0,6).join(' · ')}{apiAmenities.length > 6 ? '…' : ''}
                </div>
              )}
              {apiAmenities.length === 0 && (
                <div style={{fontSize:10,color:'var(--muted)',marginTop:4}}>
                  API chưa có amenities — điền tay vào ô trên để hiển thị.
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Title + Watermark */}
      <div className="field">
        <label className="flabel">Tiêu đề đầu video</label>
        <textarea
          className="inp body-font textarea"
          placeholder={`POV\nKhi bạn tìm được homestay xịn`}
          value={title}
          onChange={e=>set('title')(e.target.value)}
          style={{minHeight:60}}
        />
        <div className="slider-row" style={{marginTop:8}}>
          <input
            type="range"
            min="0.5"
            max="10"
            step="0.5"
            value={titleSecs}
            onChange={e=>set('titleDuration')(+e.target.value)}
          />
          <span className="slider-val">{titleSecs}s</span>
        </div>
        <div style={{fontSize:10,color:'var(--muted)',marginTop:4,fontStyle:'italic'}}>
          Tiêu đề hiển thị trước; sau đó overlay listing hiển thị đến hết video.
        </div>
      </div>

      {/* Custom title font (bring-your-own) */}
      <FontUpload
        value={settings.customFont || ''}
        onChange={(v) => set('customFont')(v)}
        previewText={(title || '').split(/[\n|]/)[0]}
      />
      <div className="field">
        <label className="flabel">Watermark</label>
        <input className="inp body-font" placeholder="© Tên của bạn" value={watermark} onChange={e=>set('watermark')(e.target.value)}/>
      </div>

      {/* ── Tự đăng social (qua webhook Make.com / n8n) ── */}
      <div className="field">
        <label className="flabel">📤 Tự đăng social</label>
        <div className="toggle-row">
          <div className="ti">
            <div className="t1">Gửi video sau khi render xong</div>
            <div className="t2">Webhook Make.com / n8n → TikTok · Facebook</div>
          </div>
          <div className={`toggle${autoPost?' on':''}`} onClick={()=>set('autoPost')(!autoPost)}/>
        </div>
        {autoPost && (
          <div style={{marginTop:8}}>
            <input
              className="inp body-font"
              placeholder="Dán link Webhook từ Make.com (https://hook…)"
              value={webhookUrl}
              onChange={e=>set('webhookUrl')(e.target.value)}
              style={{marginBottom:8}}
            />
            <div className="toggle-row">
              <div className="ti">
                <div className="t1">TikTok</div>
                <div className="t2">Đăng dạng nháp — mở app TikTok bấm publish</div>
              </div>
              <div className={`toggle${postTiktok?' on':''}`} onClick={()=>set('postTiktok')(!postTiktok)}/>
            </div>
            <div className="toggle-row">
              <div className="ti">
                <div className="t1">Facebook</div>
                <div className="t2">Đăng công khai lên Page</div>
              </div>
              <div className={`toggle${postFacebook?' on':''}`} onClick={()=>set('postFacebook')(!postFacebook)}/>
            </div>
            <div style={{fontSize:10,color:'var(--muted)',marginTop:4,fontStyle:'italic'}}>
              Render xong app tự gửi video + thông tin (tiêu đề, giá, địa chỉ, tiện nghi) tới webhook.
              Gửi lỗi sẽ báo ở thanh tiến trình nhưng KHÔNG làm hỏng video.
            </div>
          </div>
        )}
      </div>

      {/* Output mode — bấm "Tạo" xuất gì (áp dụng cả listing đơn lẫn nhiều listing) */}
      <div className="field">
        <label className="flabel">Khi bấm Tạo → xuất gì</label>
        <div className="seg" style={{flexWrap:'wrap'}}>
          {[
            ['both', '🎬+🖼️ Cả hai'],
            ['video', '🎬 Chỉ video'],
            ['thumbnail', '🖼️ Chỉ thumbnail'],
          ].map(([v,l])=>(
            <button
              key={v}
              type="button"
              className={`seg-btn${(settings.batchMode||'both')===v?' on':''}`}
              style={{flex:'1 1 30%',minWidth:'30%'}}
              onClick={()=>set('batchMode')(v)}
            >{l}</button>
          ))}
        </div>
        <div style={{fontSize:10,color:'var(--muted)',marginTop:4,fontStyle:'italic'}}>
          Áp dụng cho cả listing đơn lẫn nhiều listing. “Chỉ thumbnail” = chỉ tạo ảnh bìa, KHÔNG render video → nhanh hơn nhiều.
        </div>
      </div>

      {/* ── Thumbnail (ảnh bìa collage tĩnh) ── */}
      <div className="field">
        <label className="flabel">🖼️ Thumbnail (ảnh bìa)</label>

        <label className="flabel" style={{marginBottom:6,marginTop:4}}>Kiểu thumbnail</label>
        <div className="seg" style={{flexWrap:'wrap',marginBottom:8}}>
          {[
            ['', 'Classic'],
            ['daiky', 'Daiky'],
            ['valey', 'Valey'],
            ['peony', 'Peony'],
            ['tiger', 'Tiger'],
            ['cento', 'Cento'],
            ['amber', 'Amber'],
            ['strip', 'Strip'],
            ['creamgrid', 'CreamGrid'],
            ['filmstrip', 'Filmstrip'],
            ['random', '🎲 Random'],
          ].map(([v,l])=>(
            <button
              key={v||'classic'}
              type="button"
              className={`seg-btn${(settings.thumbTemplate||'')===v?' on':''}`}
              style={{flex:'1 1 30%',minWidth:'30%'}}
              onClick={()=>set('thumbTemplate')(v)}
            >{l}</button>
          ))}
        </div>
        <div style={{fontSize:10,color:'var(--muted)',marginBottom:8,fontStyle:'italic'}}>
          {(settings.thumbTemplate||'')===''
            ? 'Classic: 1 ảnh lớn + 3 polaroid bên phải. Để trống dữ liệu = tự lấy từ listing.'
            : (settings.thumbTemplate==='random'
                ? '🎲 Random: mỗi listing tự bốc 1 trong 9 template khác nhau.'
                : ({
                    cento: 'Lưới 2×2 + panel thông tin nâu giữa + nhãn combo 4 góc.',
                    amber: '1 ảnh hero, khối giá góc trên, tiêu đề serif lớn + pill địa chỉ.',
                    strip: 'Tiêu đề trên cùng + dải 3 ảnh ngang + giá phía dưới.',
                    creamgrid: 'Nền kem, chữ serif đỏ, lưới ảnh lệch cột + caption "Disco Room".',
                    filmstrip: '3 dải ảnh xếp dọc, dải giữa phủ chữ (tiêu đề + tagline + giá).',
                  }[settings.thumbTemplate]
                  || 'Lưới 2×2 + khối chữ giữa. Valey có bảng giá Trong tuần/Cuối tuần (tự lấy từ prices_by_week).'))}
        </div>

        <label className="flabel" style={{marginBottom:6}}>Font tiêu đề</label>
        <div className="seg" style={{marginBottom:8}}>
          {[
            ['Baloo2-Bold.ttf', 'Baloo tròn'],
            ['Baloo2-ExtraBold.ttf', 'Baloo đậm'],
            ['Prata-Regular.ttf', 'Prata serif'],
          ].map(([v,l])=>(
            <button
              key={v}
              type="button"
              className={`seg-btn${thumbTitleFont===v?' on':''}`}
              onClick={()=>set('thumbTitleFont')(v)}
            >{l}</button>
          ))}
        </div>
        <input
          className="inp body-font"
          placeholder="Tiêu đề lớn (vd Sunset) — trống = nickname"
          value={thumbTitle}
          onChange={e=>set('thumbTitle')(e.target.value)}
          style={{marginBottom:8}}
        />
        {(settings.thumbTemplate==='creamgrid' || settings.thumbTemplate==='random') && (
          <input
            className="inp body-font"
            placeholder="Chữ trên ảnh CreamGrid (vd Disco Room) — trống = bỏ"
            value={settings.thumbCaption || ''}
            onChange={e=>set('thumbCaption')(e.target.value)}
            style={{marginBottom:8}}
          />
        )}
        <input
          className="inp body-font"
          placeholder="Dòng giá (vd 2h 249k- 4h 367k- Qua đêm 449k)"
          value={thumbPrice}
          onChange={e=>set('thumbPrice')(e.target.value)}
          style={{marginBottom:8}}
        />
        <input
          className="inp body-font"
          placeholder="@handle (vd @tranhouse_hanoi)"
          value={thumbWatermark}
          onChange={e=>set('thumbWatermark')(e.target.value)}
        />
        <div className="color-grid" style={{marginTop:10}}>
          <label>
            <span>Màu viền tiêu đề</span>
            <input type="color" value={thumbStrokeColor} onChange={e=>set('thumbStrokeColor')(e.target.value)}/>
          </label>
          <label>
            <span>Màu nền data</span>
            <input type="color" value={thumbPillColor} onChange={e=>set('thumbPillColor')(e.target.value)}/>
          </label>
        </div>
        <div style={{marginTop:8}}>
          <label className="flabel">
            Độ dày viền &nbsp;
            <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{thumbStrokeWidth}px</span>
          </label>
          <div className="slider-row">
            <input type="range" min="0" max="14" step="1" value={thumbStrokeWidth} onChange={e=>set('thumbStrokeWidth')(+e.target.value)}/>
            <span className="slider-val">{thumbStrokeWidth}px</span>
          </div>
        </div>
        <div>
          <label className="flabel">
            Độ đậm nền data &nbsp;
            <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{Math.round(thumbPillAlpha*100)}%</span>
          </label>
          <div className="slider-row">
            <input type="range" min="0.05" max="0.8" step="0.05" value={thumbPillAlpha} onChange={e=>set('thumbPillAlpha')(+e.target.value)}/>
            <span className="slider-val">{Math.round(thumbPillAlpha*100)}%</span>
          </div>
        </div>
        <div>
          <label className="flabel">
            Cỡ chữ tiêu đề &nbsp;
            <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{Number(thumbTitleScale).toFixed(2)}×</span>
          </label>
          <div className="slider-row">
            <input type="range" min="0.5" max="1.6" step="0.05" value={thumbTitleScale} onChange={e=>set('thumbTitleScale')(+e.target.value)}/>
            <span className="slider-val">{Number(thumbTitleScale).toFixed(2)}×</span>
          </div>
        </div>
        <div>
          <label className="flabel">
            Cỡ chữ data &nbsp;
            <span style={{color:'var(--gold)',fontFamily:'var(--mono)'}}>{Number(thumbDataScale).toFixed(2)}×</span>
          </label>
          <div className="slider-row">
            <input type="range" min="0.6" max="1.5" step="0.05" value={thumbDataScale} onChange={e=>set('thumbDataScale')(+e.target.value)}/>
            <span className="slider-val">{Number(thumbDataScale).toFixed(2)}×</span>
          </div>
        </div>
        <div style={{fontSize:10,color:'var(--muted)',marginTop:4,fontStyle:'italic'}}>
          Địa chỉ &amp; tiện nghi lấy theo listing (tiện nghi dùng chung ô ở trên). Bấm “🖼️ Tạo Thumbnail” ở cột 3.
        </div>
      </div>

      {uploadedCount>0 && (
        <div className="summary">
          <div className="plabel" style={{marginBottom:8}}>Tóm tắt</div>
          {[
            ['Hiệu ứng', mode==='kenburns'?'Ken Burns':mode==='slideshow'?'Slideshow':'Timelapse'],
            ['Kiểu',
              currentEffect==='random'
                ? '🎲 Random / ảnh'
                : (effectOptions.find(([v])=>v===currentEffect)?.[1] || currentEffect)],
            ['Số ảnh/listing', safePhotoLimit],
            ['Mỗi ảnh', perImg+'s'],
            ['Tổng dự kiến', uploadedCount>0 ? totalTime+'s' : '—'],
            ['Tiêu đề', title ? titleSecs+'s' : 'Tắt'],
            ['Định dạng', tiktok?'1080×1920':'1920×1080'],
            ['Overlay', useOverlay?'Bật':'Tắt'],
            ['Template', template],
            ['Font listing', useOverlay ? (overlayFont==='lilita'?'Lilita One':'Playfair') : '—'],
            ['Danh mục giá', useOverlay ? `${overlayPriceTypes.length}/${priceTypeOptions.length}` : '—'],
          ].map(([k,v])=>(
            <div key={k} className="summary-row">
              <span className="k">{k}</span>
              <span className="v">{v}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function activePreviewText(settings) {
  if (settings.overlayStyle === 'bubble') return 'Phòng đẹp giá tốt'
  return 'Homestay trung tâm'
}
