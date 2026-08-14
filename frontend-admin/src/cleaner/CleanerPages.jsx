// Ba màn của cô dọn dẹp: ca hôm nay, checklist một phòng, công của tôi.
//
// Thiết kế cho điện thoại tầm trung, tay ướt, ánh đèn phòng tắm: chữ ≥15px,
// vùng chạm ≥44px, điều hướng ở đáy màn hình vừa tầm ngón cái.

import { useEffect, useMemo, useState } from 'react'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { useRouter } from '../router.jsx'
import PhotoUploader from '../components/PhotoUploader.jsx'
import { Alert, Empty, Progress, Spinner } from '../components/ui.jsx'
import {
	ALLOWANCE_LABEL,
	ROOM_TYPE_LABEL,
	SESSION_LABEL,
	dayKey,
	dayMonth,
	hour,
	money,
	monthKey,
	monthLabel,
	shiftMonth,
} from '../format.js'

export function CleanerShell({ active, title, back, children }) {
	const { logout, user } = useAuth()
	const { navigate } = useRouter()

	return (
		<div className="mob">
			<header className="mob-top">
				{back ? (
					<button className="mob-btn" onClick={() => navigate(back)} aria-label="Quay lại">‹</button>
				) : (
					<span className="mob-btn mob-btn--hidden" />
				)}
				<div className="mob-title">
					<span>{title}</span>
					{user && <small>{user.name}</small>}
				</div>
				<button
					className="mob-btn"
					aria-label="Đăng xuất"
					onClick={async () => {
						await logout()
						navigate('/login', { replace: true })
					}}
				>
					⎋
				</button>
			</header>

			<main className="mob-body">{children}</main>

			<nav className="mob-tabs">
				<button className={`mob-tab${active === 'today' ? ' active' : ''}`} onClick={() => navigate('/cleaning')}>
					<span>📋</span>
					<span>Ca hôm nay</span>
				</button>
				<button className={`mob-tab${active === 'pay' ? ' active' : ''}`} onClick={() => navigate('/cleaning/pay')}>
					<span>💵</span>
					<span>Công của tôi</span>
				</button>
			</nav>
		</div>
	)
}

// ─── Ca hôm nay ───────────────────────────────────────────────────────────

export function CleanerTodayPage() {
	const { navigate } = useRouter()
	const [sessions, setSessions] = useState(null)
	const [err, setErr] = useState('')

	useEffect(() => {
		api
			.sessions({ day: dayKey() })
			.then((d) => setSessions(d.sessions || []))
			.catch((e) => {
				setErr(e.message)
				setSessions([])
			})
	}, [])

	// Sắp theo GIỜ PHẢI XONG, không theo tên phòng: cô mở app ra để biết đi đâu
	// trước, và phòng có khách nhận lúc 14h phải nằm trên phòng trống cả chiều.
	const rows = useMemo(
		() => [...(sessions || [])].sort((a, b) => (a.deadline_at || 0) - (b.deadline_at || 0)),
		[sessions],
	)

	const remaining = rows.filter((s) => s.status === 'todo' || s.status === 'in_progress').length
	const earned = rows.reduce((sum, s) => sum + (s.pay?.total || 0), 0)

	return (
		<CleanerShell active="today" title="Ca hôm nay">
			<div className="mob-sum">
				<div>
					<strong>{remaining}</strong>
					<span>phòng còn phải dọn</span>
				</div>
				<div>
					<strong>{money(earned)}</strong>
					<span>công hôm nay</span>
				</div>
			</div>

			<Alert>{err}</Alert>

			{sessions === null ? (
				<Spinner />
			) : !rows.length ? (
				<Empty icon="☕">Hôm nay bạn chưa được xếp phòng nào. Nghỉ ngơi nhé!</Empty>
			) : (
				<div className="mob-list">
					{rows.map((s) => {
						const done = s.status === 'submitted' || s.status === 'approved'
						return (
							<button
								key={s.id}
								className={`mob-card${done ? ' mob-card--done' : ''}`}
								onClick={() => navigate(`/cleaning/session/${s.id}`)}
							>
								<div className="mob-card-top">
									<span className="mob-card-time">
										🕐 Xong trước <strong>{hour(s.deadline_at)}</strong>
									</span>
									<span className={`pill pill--${s.status}`}>{SESSION_LABEL[s.status]}</span>
								</div>

								<div className="mob-card-name">{s.room?.name || s.room_id}</div>
								<div className="mob-card-addr">📍 {s.room?.address}</div>
								<div className="mob-card-meta">
									{ROOM_TYPE_LABEL[s.room?.room_type] || s.room?.room_type} · khách trả phòng {hour(s.checkout_at)}
								</div>

								{s.guest_note && <div className="mob-note">Lưu ý: {s.guest_note}</div>}
								{s.late && <div className="mob-note mob-note--danger">Đã quá giờ khách vào</div>}

								<div className="mob-card-foot">
									<Progress percent={s.progress?.percent} />
									<span>
										{s.progress?.done_required}/{s.progress?.total_required} việc
									</span>
									<strong>{money(s.pay?.payable ? s.pay.total : s.base_fee)}</strong>
								</div>

								{s.status === 'rejected' && s.review_note && (
									<div className="mob-note mob-note--danger">Quản lý nhắn: {s.review_note}</div>
								)}
							</button>
						)
					})}
				</div>
			)}
		</CleanerShell>
	)
}

// ─── Checklist một phòng ──────────────────────────────────────────────────

export function CleanerSessionPage({ sessionId }) {
	const [session, setSession] = useState(null)
	const [err, setErr] = useState('')
	const [openGroup, setOpenGroup] = useState('')
	const [allowanceOpen, setAllowanceOpen] = useState(false)

	useEffect(() => {
		api
			.session(sessionId)
			.then((d) => setSession(d.session))
			.catch((e) => setErr(e.message))
	}, [sessionId])

	const template = session?.template_snapshot
	const locked = session?.status === 'approved'

	// Lưu NGAY từng ảnh, không có nút "Lưu": cô đi sang phòng khác, màn hình khoá,
	// trình duyệt bị thu hồi — state chờ bấm Lưu là mất trắng.
	async function saveItem(itemId, patch) {
		try {
			const d = await api.saveItem(sessionId, itemId, patch)
			setSession(d.session)
			setErr('')
		} catch (e) {
			setErr(e.message)
		}
	}

	if (err && !session) {
		return (
			<CleanerShell active="today" title="Không mở được" back="/cleaning">
				<Alert>{err}</Alert>
			</CleanerShell>
		)
	}
	if (!session) {
		return (
			<CleanerShell active="today" title="Đang tải…" back="/cleaning">
				<Spinner />
			</CleanerShell>
		)
	}

	const p = session.progress || {}
	const submitted = session.status === 'submitted' || session.status === 'approved'

	return (
		<CleanerShell active="today" title={session.room?.name || 'Ca dọn'} back="/cleaning">
			<div className="mob-head">
				<div>📍 {session.room?.address}</div>
				<div>
					Khách trả phòng {hour(session.checkout_at)} · xong trước <strong>{hour(session.deadline_at)}</strong>
				</div>
				{session.room?.door_note && <div className="mob-door">🔑 {session.room.door_note}</div>}
				{session.guest_note && <div className="mob-note">Lưu ý: {session.guest_note}</div>}
			</div>

			{submitted && (
				<div className="banner banner--ok">
					<strong>Đã đủ ảnh — công của bạn đã được ghi nhận.</strong>
					<span>
						{session.status === 'approved'
							? `Quản lý đã duyệt: ${money(session.pay?.total)}.`
							: `${money(session.pay?.total)} đang chờ quản lý đối soát.`}
					</span>
				</div>
			)}

			{session.status === 'rejected' && (
				<div className="banner banner--danger">
					<strong>Quản lý chưa duyệt ca này.</strong>
					<span>{session.review_note || 'Vui lòng liên hệ quản lý.'}</span>
				</div>
			)}

			<Alert>{err}</Alert>

			<div className="groups">
				{(template?.groups || []).map((g) => {
					const items = g.items || []
					const doneCount = items.filter((it) => isDone(session, it)).length
					const allDone = items.length > 0 && doneCount === items.length
					const open = openGroup === g.id
					return (
						<section key={g.id} className={`group${allDone ? ' group--done' : ''}`}>
							<button className="group-head" onClick={() => setOpenGroup(open ? '' : g.id)} aria-expanded={open}>
								<span className="group-ico">{allDone ? '✅' : '⬜'}</span>
								<span className="group-title">{g.title}</span>
								<span className="group-count">
									{doneCount}/{items.length}
								</span>
								<span>{open ? '⌄' : '›'}</span>
							</button>

							{open && (
								<div className="group-body">
									{items.map((it) => {
										const photos = session.items_state?.[it.id]?.photos || []
										const checked = !!session.items_state?.[it.id]?.checked
										const done = isDone(session, it)
										return (
											<div key={it.id} className={`item${done ? ' item--done' : ''}`}>
												<div className="item-head">
													<span className="item-ico">{done ? '✅' : '⬜'}</span>
													<div>
														<div className="item-title">{it.title}</div>
														{it.hint && <div className="item-hint">{it.hint}</div>}
														{it.require_photo && (
															<div className="item-req">
																Cần {it.min_photos || 1} ảnh{photos.length ? ` · đã có ${photos.length}` : ''}
															</div>
														)}
													</div>
												</div>

												{it.require_photo ? (
													<PhotoUploader
														photos={photos}
														disabled={locked}
														onChange={(next) => saveItem(it.id, { photos: next })}
													/>
												) : (
													<label className="tick">
														<input
															type="checkbox"
															checked={checked}
															disabled={locked}
															onChange={(e) => saveItem(it.id, { checked: e.target.checked })}
														/>
														<span>Đã làm xong</span>
													</label>
												)}
											</div>
										)
									})}
								</div>
							)}
						</section>
					)
				})}
			</div>

			<div className="allow-box">
				<div className="allow-head">
					<strong>Phụ cấp phát sinh</strong>
					{!locked && (
						<button className="btn btn--ghost" onClick={() => setAllowanceOpen(true)}>
							+ Đề nghị
						</button>
					)}
				</div>
				{(session.allowances || []).length ? (
					session.allowances.map((a) => (
						<div key={a.id} className="allow-row">
							<span>{a.type}</span>
							<span>{money(a.amount)}</span>
							<span className={`pill pill--${a.status}`}>{ALLOWANCE_LABEL[a.status]}</span>
						</div>
					))
				) : (
					<p className="help">
						Có việc ngoài checklist (giặt chăn ga, khách để quá bẩn…)? Bấm Đề nghị và chụp ảnh, quản lý sẽ duyệt thêm
						tiền.
					</p>
				)}
			</div>

			<div className="mob-bottom">
				{p.complete ? (
					<div className="mob-bottom-ok">✅ Xong hết rồi — {money(session.pay?.payable ? session.pay.total : session.base_fee)}</div>
				) : (
					<div className="mob-bottom-todo">
						<Progress percent={p.percent} />
						{/* Nói TÊN mục còn thiếu, không phải "chưa đủ điều kiện": cô cần biết
						    phải quay lại phòng nào, không cần biết hệ thống nghĩ gì. */}
						<span>
							Còn <strong>{(p.missing || []).length}</strong> việc cần ảnh
							{(p.missing || []).length ? `: ${p.missing[0]}` : ''}
						</span>
					</div>
				)}
			</div>

			{allowanceOpen && (
				<AllowanceModal
					sessionId={sessionId}
					onClose={() => setAllowanceOpen(false)}
					onSaved={(s) => {
						setSession(s)
						setAllowanceOpen(false)
					}}
				/>
			)}
		</CleanerShell>
	)
}

function isDone(session, item) {
	const st = session.items_state?.[item.id] || {}
	if (!item.require_photo) return !!st.checked
	return (st.photos || []).length >= (item.min_photos || 1)
}

function AllowanceModal({ sessionId, onClose, onSaved }) {
	const [types, setTypes] = useState([])
	const [type, setType] = useState('bed_linen')
	const [amount, setAmount] = useState('30000')
	const [note, setNote] = useState('')
	const [photos, setPhotos] = useState([])
	const [err, setErr] = useState('')
	const [busy, setBusy] = useState(false)

	useEffect(() => {
		api.meta().then((d) => setTypes(d.allowance_types || [])).catch(() => {})
	}, [])

	async function submit() {
		setBusy(true)
		setErr('')
		try {
			const d = await api.addAllowance({
				session_id: sessionId,
				type,
				amount: Number(amount) || 0,
				note,
				photos,
			})
			onSaved(d.session)
		} catch (e) {
			setErr(e.message)
		} finally {
			setBusy(false)
		}
	}

	return (
		<div className="modal" onClick={onClose}>
			<div className="modal-in" onClick={(e) => e.stopPropagation()}>
				<h2>Đề nghị phụ cấp</h2>

				<label className="field">
					<span>Loại việc phát sinh</span>
					<select
						value={type}
						onChange={(e) => {
							setType(e.target.value)
							const t = types.find((x) => x.key === e.target.value)
							setAmount(t?.default_amount ? String(t.default_amount) : '')
						}}
					>
						{types.map((t) => (
							<option key={t.key} value={t.key}>
								{t.label}
							</option>
						))}
					</select>
				</label>

				<label className="field">
					<span>Số tiền đề nghị (đ)</span>
					<input type="number" inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value)} />
				</label>

				<label className="field">
					<span>Mô tả cho quản lý</span>
					<input placeholder="VD: chăn dính bẩn phải giặt riêng" value={note} onChange={(e) => setNote(e.target.value)} />
				</label>

				<div className="field">
					<span>Ảnh chứng minh</span>
					<PhotoUploader photos={photos} max={3} onChange={setPhotos} compact />
				</div>

				<Alert>{err}</Alert>

				<div className="modal-actions">
					<button className="btn btn--ghost" onClick={onClose}>
						Huỷ
					</button>
					<button className="btn btn--primary" disabled={busy || !Number(amount)} onClick={submit}>
						{busy ? 'Đang gửi…' : 'Gửi đề nghị'}
					</button>
				</div>
			</div>
		</div>
	)
}

// ─── Công của tôi ─────────────────────────────────────────────────────────

export function CleanerPayPage() {
	const { navigate } = useRouter()
	const [month, setMonth] = useState(monthKey())
	const [data, setData] = useState(null)
	const [err, setErr] = useState('')

	useEffect(() => {
		setData(null)
		api
			.timesheet(month)
			.then(setData)
			.catch((e) => {
				setErr(e.message)
				setData({ rows: [], sessions: [] })
			})
	}, [month])

	const row = data?.rows?.[0]
	const sessions = useMemo(
		() => [...(data?.sessions || [])].sort((a, b) => (b.checkout_at || 0) - (a.checkout_at || 0)),
		[data],
	)
	const thisMonth = monthKey()

	return (
		<CleanerShell active="pay" title="Công của tôi">
			<div className="mob-month">
				<button onClick={() => setMonth(shiftMonth(month, -1))} aria-label="Tháng trước">‹</button>
				<span>{monthLabel(month)}</span>
				<button disabled={month >= thisMonth} onClick={() => setMonth(shiftMonth(month, 1))} aria-label="Tháng sau">
					›
				</button>
			</div>

			<div className="pay-total">
				<span>Tổng công {monthLabel(month).toLowerCase()}</span>
				<strong>{money(row?.total)}</strong>
				<small>
					{row?.rooms || 0} phòng · {money(row?.confirmed_total)} đã chốt
					{row?.provisional_total ? ` · ${money(row.provisional_total)} chờ quản lý duyệt` : ''}
				</small>
			</div>

			{!!row?.allowance_pending && (
				<div className="banner banner--warn">
					<strong>{money(row.allowance_pending)} phụ cấp đang chờ duyệt</strong>
					<span>Khoản này chưa được cộng vào tổng ở trên.</span>
				</div>
			)}

			<Alert>{err}</Alert>

			{data === null ? (
				<Spinner />
			) : !sessions.length ? (
				<Empty icon="💵">Tháng này chưa có phòng nào được ghi công.</Empty>
			) : (
				<div className="paylist">
					{sessions.map((s) => (
						<div key={s.id} className="payrow" onClick={() => navigate(`/cleaning/session/${s.id}`)}>
							<div className="payrow-date">{dayMonth(s.checkout_at)}</div>
							<div className="payrow-main">
								<div>{s.room?.name || s.room_id}</div>
								<span className={`pill pill--${s.status}`}>{SESSION_LABEL[s.status]}</span>
								{!!s.pay?.deduction && <span className="payrow-ded">bị trừ {money(s.pay.deduction)}</span>}
							</div>
							<div className="payrow-money">{s.status === 'rejected' ? '—' : money(s.pay?.total)}</div>
						</div>
					))}
				</div>
			)}

			<p className="help">
				Số tiền ở đây là công dọn phòng, chưa gồm khoản quản lý trả riêng. Có gì chưa đúng, nhắn quản lý kèm ngày và tên
				phòng.
			</p>
		</CleanerShell>
	)
}
