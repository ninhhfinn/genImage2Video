// Màn chụp checklist — một chạm.
//
// VÌ SAO VIẾT LẠI: bản trước bắt cô mở từng nhóm việc, bấm chụp, bấm xong, rồi
// bấm sang nhóm khác. Cô dọn dẹp phải xong căn này để sang căn khác, không có
// thời gian cho ngần ấy nút — và khi checklist tốn thời gian hơn việc dọn thì
// người ta bỏ checklist chứ không bỏ việc dọn.
//
// LUỒNG MỚI: chạm vào phòng là vào thẳng đây. Một mục việc = một màn = một nút
// chụp. Chụp xong TỰ sang mục tiếp theo. Hết mục thì tự ghi nhận hoàn tất.
// Không có nút "đã xong", không có nút "tiếp theo", không phải chọn nhóm.
//
// HAI ĐIỀU LÀM CHO NÓ NHANH THẬT:
//
//  1. Không chờ mạng. Chụp xong là sang mục sau NGAY, ảnh gửi lên chạy nền. Nhà
//     bê tông sóng yếu mà mỗi ảnh chờ 3-4 giây thì 15 mục là gần một phút đứng
//     nhìn màn hình.
//  2. Không có nút bỏ qua. Checklist dựng riêng cho từng loại phòng nên không có
//     mục "không áp dụng"; khách để đồ chắn thì chụp chỗ bị chắn — đó chính là
//     lời giải thích.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, photoSrc } from '../api.js'
import { minutes } from '../format.js'

const ACCEPT = 'image/png,image/jpeg,image/webp'
const MAX_EDGE = 1600
const RESIZE_ABOVE = 400 * 1024

/** Thu nhỏ ảnh trước khi gửi. Lỗi thì gửi bản gốc — thà tốn dữ liệu hơn mất ảnh. */
async function shrink(file) {
	if (file.size <= RESIZE_ABOVE) return file
	try {
		const bitmap = await createImageBitmap(file)
		const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height))
		if (scale >= 1) return file
		const canvas = document.createElement('canvas')
		canvas.width = Math.round(bitmap.width * scale)
		canvas.height = Math.round(bitmap.height * scale)
		canvas.getContext('2d').drawImage(bitmap, 0, 0, canvas.width, canvas.height)
		const blob = await new Promise((r) => canvas.toBlob(r, 'image/jpeg', 0.82))
		return blob ? new File([blob], 'anh.jpg', { type: 'image/jpeg' }) : file
	} catch {
		return file
	}
}

function flatItems(template) {
	const out = []
	;(template?.groups || []).forEach((g) => (g.items || []).forEach((it) => out.push({ ...it, group: g.title })))
	return out
}

const minPhotos = (it) => Math.max(1, it.min_photos || 1)

export default function CaptureFlow({ session, onSessionChange, onExit }) {
	const items = useMemo(() => flatItems(session.template_snapshot), [session])

	// shots: itemId → [{ preview, url, state: 'sending'|'ok'|'failed', file }]
	const [shots, setShots] = useState(() => {
		const init = {}
		items.forEach((it) => {
			const saved = session.items_state?.[it.id]?.photos || []
			if (saved.length) init[it.id] = saved.map((p) => ({ preview: photoSrc(p.url), url: p.url, state: 'ok' }))
		})
		return init
	})

	const doneCount = (id) => (shots[id] || []).filter((s) => s.state !== 'failed').length
	const isDone = useCallback((it) => doneCount(it.id) >= minPhotos(it), [shots])

	// Vào thẳng mục chưa xong đầu tiên — quay lại giữa chừng thì tiếp đúng chỗ bỏ dở.
	const [idx, setIdx] = useState(() => {
		const i = items.findIndex((it) => {
			const saved = session.items_state?.[it.id]?.photos || []
			return saved.length < minPhotos(it)
		})
		return i < 0 ? items.length : i
	})
	// Ba màn rõ ràng thay vì suy từ chỉ số: 'shoot' đang chụp, 'review' xem lại
	// ảnh, 'done' đã xong. Trước đó tôi suy màn từ idx và nút "Xem lại" rơi ngược
	// vào màn Xong.
	const [view, setView] = useState('shoot')
	const [note, setNote] = useState('')
	const fileRef = useRef(null)

	const remaining = items.filter((it) => !isDone(it)).length
	const total = items.length
	const current = idx < items.length ? items[idx] : null

	// Bấm Bắt đầu là một chạm thừa: mở màn chụp tức là đã bắt đầu dọn.
	useEffect(() => {
		if (!session.started_at) api.startSession(session.id).catch(() => {})
	}, [session.id, session.started_at])

	/** Gửi một ảnh lên, chạy nền. Không chặn luồng chụp. */
	const upload = useCallback(
		async (item, shotId, file) => {
			try {
				const sent = await shrink(file)
				const res = await api.uploadPhoto(sent)
				let urls = []
				setShots((prev) => {
					const list = (prev[item.id] || []).map((s) => (s.id === shotId ? { ...s, url: res.url, state: 'ok' } : s))
					urls = list.filter((s) => s.url).map((s) => ({ url: s.url }))
					return { ...prev, [item.id]: list }
				})
				// Ghi vào ca sau khi đã có URL. Gửi cả mảng vì API thay nguyên mục.
				const d = await api.saveItem(session.id, item.id, { photos: urls })
				if (d?.session) onSessionChange?.(d.session)
			} catch {
				setShots((prev) => ({
					...prev,
					[item.id]: (prev[item.id] || []).map((s) => (s.id === shotId ? { ...s, state: 'failed' } : s)),
				}))
			}
		},
		[session.id, onSessionChange],
	)

	function advanceFrom(item) {
		// Đủ ảnh cho mục này thì sang mục CHƯA xong tiếp theo, không phải mục kế bên:
		// cô quay lại chụp bù một mục ở giữa xong phải nhảy tới chỗ còn thiếu.
		const need = minPhotos(item)
		const have = doneCount(item.id) + 1
		if (have < need) return
		const next = items.findIndex((it, i) => i > idx && !isDone(it))
		if (next >= 0) {
			setIdx(next)
			return
		}
		const anyLeft = items.findIndex((it) => it.id !== item.id && !isDone(it))
		if (anyLeft >= 0) {
			setIdx(anyLeft)
			return
		}
		setView('done')
	}

	function onPick(e) {
		const file = e.target.files?.[0]
		e.target.value = ''
		if (!file || !current) return

		const shotId = `${Date.now()}-${Math.random()}`
		const preview = URL.createObjectURL(file)
		setShots((prev) => ({
			...prev,
			[current.id]: [...(prev[current.id] || []), { id: shotId, preview, state: 'sending' }],
		}))
		// Sang mục sau NGAY, không đợi mạng.
		advanceFrom(current)
		upload(current, shotId, file)
	}

	function retake(item) {
		setShots((prev) => ({ ...prev, [item.id]: [] }))
		api.saveItem(session.id, item.id, { photos: [] }).catch(() => {})
		const i = items.findIndex((x) => x.id === item.id)
		if (i >= 0) setIdx(i)
		setView('shoot')
	}

	const sending = Object.values(shots).flat().filter((s) => s.state === 'sending').length
	const failed = Object.values(shots).flat().filter((s) => s.state === 'failed').length
	const allDone = items.length > 0 && items.every(isDone)

	// ─── Màn xong ───────────────────────────────────────────────────────────
	if (view === 'done') {
		return (
			<div className="cap cap--done">
				<div className="cap-done-mark">✅</div>
				<h1>Xong rồi!</h1>
				<p className="cap-done-sub">
					{items.length} việc · {Object.values(shots).flat().length} ảnh
					{session.minutes ? ` · ${minutes(session.minutes)}` : ''}
				</p>

				{sending > 0 && <div className="cap-note">Đang gửi {sending} ảnh… cứ để máy chạy nền, không cần chờ.</div>}
				{failed > 0 && (
					<div className="cap-note cap-note--bad">
						{failed} ảnh chưa gửi được. Kiểm tra sóng rồi chụp lại mục đó.
					</div>
				)}

				<button className="cap-big cap-big--primary" onClick={onExit}>
					Sang phòng tiếp theo
				</button>
				<button
					className="cap-link"
					onClick={() => {
						setFinished(false)
						setIdx(items.length) // màn xem lại
					}}
				>
					Xem lại ảnh đã chụp
				</button>

				{/* Báo hỏng hóc nằm NGOÀI luồng chụp — nó không phải một mục checklist,
				    và bắt cô đi qua nó mỗi ca chỉ để bấm "không có gì" là thêm một chạm
				    vô ích cho 95% số ca. */}
				<div className="cap-report">
					<label>Có gì hỏng hoặc thiếu đồ? (không bắt buộc)</label>
					<input
						placeholder="VD: vòi sen rỉ nước, thiếu 1 khăn tắm"
						value={note}
						onChange={(e) => setNote(e.target.value)}
						onBlur={() => note.trim() && api.reportIssue(session.id, note).catch(() => {})}
					/>
				</div>
			</div>
		)
	}

	// ─── Màn xem lại ────────────────────────────────────────────────────────
	if (view === 'review' || !current) {
		return (
			<div className="cap cap--review">
				<div className="cap-top">
					<button className="cap-x" onClick={onExit} aria-label="Đóng">✕</button>
					<span>Ảnh đã chụp</span>
					<button className="cap-x" onClick={() => setView('done')} aria-label="Xong">✓</button>
				</div>
				<div className="cap-grid">
					{items.map((it) => (
						<button key={it.id} className="cap-cell" onClick={() => retake(it)}>
							{(shots[it.id] || [])[0] ? (
								<img src={(shots[it.id] || [])[0].preview} alt="" />
							) : (
								<span className="cap-cell-empty">chưa có</span>
							)}
							<span className="cap-cell-label">{it.title}</span>
							<span className="cap-cell-retake">Chụp lại</span>
						</button>
					))}
				</div>
			</div>
		)
	}

	// ─── Màn chụp ───────────────────────────────────────────────────────────
	const need = minPhotos(current)
	const have = doneCount(current.id)
	const last = items[idx - 1]

	return (
		<div className="cap">
			<div className="cap-top">
				<button className="cap-x" onClick={onExit} aria-label="Thoát">✕</button>
				<div className="cap-progress">
					<div className="cap-bar">
						<div className="cap-bar-fill" style={{ width: `${((total - remaining) / total) * 100}%` }} />
					</div>
					<span>
						Còn {remaining}/{total} việc
					</span>
				</div>
				<span className="cap-x cap-x--ghost" />
			</div>

			{/* Ảnh vừa chụp hiện ở đây kèm nút chụp lại — chụp trượt thì sửa ngay,
			    không phải nhớ để cuối buổi quay lại. */}
			{last && (
				<button className="cap-last" onClick={() => retake(last)}>
					{(shots[last.id] || []).slice(-1)[0]?.preview && (
						<img src={(shots[last.id] || []).slice(-1)[0].preview} alt="" />
					)}
					<span>
						Vừa chụp: {last.title}
						<em>Chụp lại</em>
					</span>
				</button>
			)}

			<div className="cap-main">
				<div className="cap-group">{current.group}</div>
				<h1 className="cap-title">{current.title}</h1>
				{current.hint && <p className="cap-hint">{current.hint}</p>}
				{need > 1 && (
					<p className="cap-count">
						Ảnh {have + 1} / {need}
					</p>
				)}
			</div>

			<button className="cap-big cap-big--shoot" onClick={() => fileRef.current?.click()}>
				<span className="cap-cam">📷</span>
				Chụp ảnh
			</button>

			<input ref={fileRef} type="file" accept={ACCEPT} capture="environment" hidden onChange={onPick} />
		</div>
	)
}
