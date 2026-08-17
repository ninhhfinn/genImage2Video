// Form báo vấn đề — dùng chung cho cả màn của cô dọn dẹp và màn quản lý.
//
// Thiết kế cho người đang đứng trong phòng, không phải ngồi bàn: loại vấn đề là
// nút to bấm một chạm chứ không phải danh sách xổ, và chọn loại xong thì mức độ
// tự gợi ý — điện nước hỏng lúc 11h trưa mà để "bình thường" là mất khách, còn
// bắt cô tự đánh giá mức độ thì phần lớn sẽ để mặc định.

import { useEffect, useState } from 'react'
import { api } from '../api.js'
import PhotoUploader from './PhotoUploader.jsx'
import { Alert } from './ui.jsx'

export default function IssueForm({ roomId, sessionId, rooms = [], onDone, onCancel }) {
	const [cats, setCats] = useState([])
	const [room, setRoom] = useState(roomId || '')
	const [category, setCategory] = useState('')
	const [urgency, setUrgency] = useState('normal')
	const [touchedUrgency, setTouchedUrgency] = useState(false)
	const [desc, setDesc] = useState('')
	const [photos, setPhotos] = useState([])
	const [busy, setBusy] = useState(false)
	const [err, setErr] = useState('')

	useEffect(() => {
		api.issues('open=1').then((d) => setCats(d.categories || [])).catch(() => {})
	}, [])

	function pickCategory(c) {
		setCategory(c.key)
		// Gợi ý mức khẩn cho loại thường chặn khách vào — nhưng nếu cô đã tự chọn
		// mức độ rồi thì tôn trọng lựa chọn đó, không ghi đè.
		if (!touchedUrgency) setUrgency(c.suggest_urgent ? 'urgent' : 'normal')
	}

	async function submit() {
		setErr('')
		if (!room) return setErr('Chưa chọn phòng.')
		if (!category) return setErr('Chưa chọn loại vấn đề.')
		if (!desc.trim()) return setErr('Viết vài chữ mô tả để người sửa biết mang theo gì.')
		setBusy(true)
		try {
			const d = await api.createIssue({
				room_id: room,
				session_id: sessionId || '',
				category,
				urgency,
				description: desc.trim(),
				photos,
			})
			onDone?.(d.issue)
		} catch (e) {
			setErr(e.message)
		} finally {
			setBusy(false)
		}
	}

	return (
		<div className="issue-form">
			{!roomId && (
				<label className="field">
					<span>Phòng</span>
					<select value={room} onChange={(e) => setRoom(e.target.value)}>
						<option value="">— Chọn phòng —</option>
						{rooms.map((r) => (
							<option key={r.id} value={r.id}>
								{r.code} · {r.name}
							</option>
						))}
					</select>
				</label>
			)}

			<div className="field">
				<span>Vấn đề gì?</span>
				<div className="cat-grid">
					{cats.map((c) => (
						<button
							key={c.key}
							type="button"
							className={`cat${category === c.key ? ' active' : ''}`}
							onClick={() => pickCategory(c)}
						>
							{c.label}
						</button>
					))}
				</div>
			</div>

			<div className="field">
				<span>Mức độ</span>
				<div className="urg-row">
					<button
						type="button"
						className={`urg${urgency === 'urgent' ? ' urg--on-urgent' : ''}`}
						onClick={() => {
							setUrgency('urgent')
							setTouchedUrgency(true)
						}}
					>
						🔴 Khẩn — chặn khách vào
					</button>
					<button
						type="button"
						className={`urg${urgency === 'normal' ? ' urg--on-normal' : ''}`}
						onClick={() => {
							setUrgency('normal')
							setTouchedUrgency(true)
						}}
					>
						Bình thường
					</button>
				</div>
			</div>

			<label className="field">
				<span>Mô tả</span>
				<textarea
					rows="3"
					placeholder="VD: vòi sen phòng tắm rỉ nước liên tục, khoá vặn không chặt"
					value={desc}
					onChange={(e) => setDesc(e.target.value)}
				/>
			</label>

			<div className="field">
				<span>Ảnh (nên có — người sửa biết mang đồ gì đi)</span>
				<PhotoUploader photos={photos} max={4} onChange={setPhotos} compact />
			</div>

			<Alert>{err}</Alert>

			<div className="modal-actions">
				{onCancel && (
					<button className="btn btn--ghost" onClick={onCancel}>
						Huỷ
					</button>
				)}
				<button className="btn btn--primary" disabled={busy} onClick={submit}>
					{busy ? 'Đang gửi…' : 'Gửi báo cáo'}
				</button>
			</div>
		</div>
	)
}
