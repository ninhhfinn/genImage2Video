import { useState, useEffect } from 'react'
import { fetchTemplates } from '../api/templates'

const EMOJI = {
  random:  '🎲',
  daiky:   '🏡',
  sunset:  '🌅',
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

  return (
    <div className="field">
      <label className="flabel">Mẫu thiết kế (Template)</label>
      {err && <div style={{fontSize:10, color:'var(--red)', marginBottom:4}}>{err}</div>}
      <div className="template-grid">
        {[RANDOM, ...templates].map(t => (
          <button
            type="button"
            key={t.name}
            className={`template-card${value === t.name ? ' on' : ''}`}
            onClick={() => onChange(t.name)}
          >
            <div className="template-thumb">{EMOJI[t.name] || '🎨'}</div>
            <div className="template-name">{t.display_name}</div>
            <div className="template-desc">{t.description}</div>
          </button>
        ))}
      </div>
      <div style={{fontSize:10, color:'var(--muted)', marginTop:6, fontStyle:'italic'}}>
        Mỗi template = bộ font/màu/layout riêng. Chọn để xem hiệu ứng khi render.
      </div>
    </div>
  )
}
