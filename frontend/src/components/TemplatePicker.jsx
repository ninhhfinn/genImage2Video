import { useState, useEffect } from 'react'
import { fetchTemplates } from '../api/templates'

// Ảnh nhận diện từng template (render mẫu theo thiết kế Canva gốc).
// Thiếu ảnh → ô gradient + chữ cái đầu.
const IMG = {
  daiky:      '/templates/video-daiky.jpg',
  sunset:     '/templates/video-sunset.jpg',
  chillgreen: '/templates/video-chillgreen.jpg',
  creampill:  '/templates/video-creampill.jpg',
  editorial:  '/templates/video-editorial.jpg',
  goldserif:  '/templates/video-goldserif.jpg',
  marquee:    '/templates/video-marquee.jpg',
  staycation: '/templates/video-staycation.jpg',
  amorex:     '/templates/video-amorex.jpg',
  ntgroom:    '/templates/video-ntgroom.jpg',
}

// "Random" = mẫu ảo: không phải file template, backend tự bốc 1 mẫu thật cho mỗi video.
const RANDOM = { name: 'random', display_name: 'Random', description: 'Mỗi video tự chọn ngẫu nhiên 1 mẫu' }

export default function TemplatePicker({ value, onChange }) {
  const [templates, setTemplates] = useState([])
  const [err, setErr] = useState('')

  useEffect(() => {
    fetchTemplates()
      .then(setTemplates)
      .catch(e => setErr(e.message || 'Không tải được template'))
  }, [])

  const all = [RANDOM, ...templates]
  const active = all.find(t => t.name === value)

  return (
    <div className="field">
      <label className="flabel">Mẫu thiết kế video</label>
      {err && <div className="tpl-err">{err}</div>}
      <div className="tpl-grid">
        {all.map(t => (
          <button
            type="button"
            key={t.name}
            title={t.description}
            className={`tpl-card ratio-video${value === t.name ? ' on' : ''}`}
            onClick={() => onChange(t.name)}
          >
            {t.name === 'random'
              ? <span className="tpl-dice">🎲</span>
              : IMG[t.name]
                ? <img src={IMG[t.name]} alt={t.display_name} loading="lazy" />
                : <span className="tpl-dice">{(t.display_name || '?')[0]}</span>}
            <span className="tpl-name">{t.display_name}</span>
            <span className="tpl-check">✓</span>
          </button>
        ))}
      </div>
      {active && <div className="sect-sub">✓ {active.display_name} — {active.description}</div>}
    </div>
  )
}
