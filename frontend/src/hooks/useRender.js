import { useState, useRef, useEffect } from 'react'
import { startRender, getStatus } from '../api/client'

export function useRender(onDone, onError) {
  const [rendering, setRendering] = useState(false)
  const [done, setDone]           = useState(false)
  const [pct, setPct]             = useState(0)
  const [progText, setProgText]   = useState('')
  const [status, setStatus]       = useState('')
  const [error, setError]         = useState('')
  const pollRef = useRef(null)

  useEffect(() => () => clearInterval(pollRef.current), [])

  const render = async (cfg) => {
    setRendering(true); setDone(false)
    setPct(5); setProgText(''); setStatus('Đang khởi động...'); setError('')
    try {
      await startRender(cfg)
      pollRef.current = setInterval(async () => {
        try {
          const { data } = await getStatus()
          setProgText(data.progress || '')
          if (data.error) {
            clearInterval(pollRef.current)
            setRendering(false); setError(data.error); setStatus('❌ Lỗi'); setPct(0)
            onError?.(data.error)   // let a queue advance past this failed item
            return
          }
          if (data.running && !data.done) {
            setPct(p => Math.min(p + 1.2, 92))
            setStatus('Đang render...')
          }
          if (data.done && !data.error) {
            clearInterval(pollRef.current)
            setPct(100); setRendering(false); setDone(true); setStatus('✅ Xong!')
            onDone?.()
          }
        } catch {}
      }, 600)
    } catch (e) {
      setRendering(false); setError(e.message)
      onError?.(e.message)
    }
  }

  return { render, rendering, done, pct, progText, status, error }
}
