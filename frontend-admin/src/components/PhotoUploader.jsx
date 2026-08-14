// Ô chụp / tải ảnh cho một mục checklist.
//
// Người dùng là cô dọn dẹp đứng trong phòng cầm điện thoại, không phải nhân viên
// ngồi máy tính. Ba hệ quả:
//
//  1. Nút "Chụp ảnh" dùng capture='environment' để bung thẳng camera sau, không
//     bắt lội qua trình chọn file.
//  2. Ảnh gửi lên NGAY khi chọn, không có nút "Lưu". Cô đi từ phòng ngủ sang
//     phòng tắm, điện thoại khoá màn hình, trình duyệt bị hệ điều hành thu hồi —
//     công sức nằm trong state chờ bấm Lưu là mất trắng.
//  3. Ảnh được thu nhỏ trước khi gửi. Ảnh gốc 5MB × 14 mục × 8 phòng/ngày là hoá
//     đơn 3G của cô, không phải của công ty.

import { useRef, useState } from 'react'
import { api, photoSrc } from '../api.js'

const ACCEPT = 'image/png,image/jpeg,image/webp'
const MAX_EDGE = 1600
const RESIZE_ABOVE = 400 * 1024

/** Thu nhỏ ảnh bằng canvas. Lỗi thì trả file gốc — thà tốn dữ liệu còn hơn mất ảnh. */
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
		if (!blob) return file
		return new File([blob], file.name.replace(/\.\w+$/, '') + '.jpg', { type: 'image/jpeg' })
	} catch {
		return file
	}
}

export default function PhotoUploader({ photos = [], onChange, max = 6, disabled = false, compact = false }) {
	const [busy, setBusy] = useState(false)
	const [note, setNote] = useState('')
	const [preview, setPreview] = useState('')
	const cameraRef = useRef(null)
	const fileRef = useRef(null)

	const full = photos.length >= max

	async function handleFiles(e) {
		const files = Array.from(e.target.files || [])
		e.target.value = ''
		if (!files.length) return

		setBusy(true)
		setNote('')
		const room = Math.max(0, max - photos.length)
		const picked = files.slice(0, room)
		const added = []
		let failed = 0

		for (const file of picked) {
			try {
				const sent = await shrink(file)
				const res = await api.uploadPhoto(sent)
				added.push({ url: res.url, uploaded_at: res.uploaded_at })
			} catch {
				failed++
			}
		}

		setBusy(false)
		if (failed) setNote(`${failed} ảnh chưa gửi được (mạng yếu). Bấm chụp lại giúp nhé.`)
		else if (files.length > picked.length) setNote(`Mỗi mục chỉ nhận tối đa ${max} ảnh.`)
		if (added.length) onChange([...photos, ...added])
	}

	return (
		<div className="ph">
			<div className="ph-grid">
				{photos.map((p, i) => (
					<div className="ph-item" key={p.url + i}>
						<img src={photoSrc(p.url)} alt="" onClick={() => setPreview(photoSrc(p.url))} />
						{!disabled && (
							<button
								type="button"
								className="ph-del"
								aria-label="Xoá ảnh"
								onClick={() => onChange(photos.filter((_, j) => j !== i))}
							>
								✕
							</button>
						)}
					</div>
				))}

				{!disabled && !full && (
					<>
						<button type="button" className="ph-add" onClick={() => cameraRef.current?.click()} disabled={busy}>
							{busy ? <span className="spin" /> : <span className="ph-ico">📷</span>}
							<span>{busy ? 'Đang gửi…' : 'Chụp ảnh'}</span>
						</button>
						{!compact && (
							<button type="button" className="ph-add ph-add--ghost" onClick={() => fileRef.current?.click()} disabled={busy}>
								<span className="ph-ico">🖼️</span>
								<span>Chọn ảnh</span>
							</button>
						)}
					</>
				)}
			</div>

			{note && <p className="ph-note">{note}</p>}

			<input ref={cameraRef} type="file" accept={ACCEPT} capture="environment" hidden onChange={handleFiles} />
			<input ref={fileRef} type="file" accept={ACCEPT} multiple hidden onChange={handleFiles} />

			{preview && (
				<div className="lightbox" onClick={() => setPreview('')}>
					<img src={preview} alt="" />
					<button type="button" className="lightbox-x" aria-label="Đóng">✕</button>
				</div>
			)}
		</div>
	)
}
