// Ba màn của cô dọn dẹp: ca hôm nay, checklist một phòng, kết quả của tôi.
//
// Thiết kế cho điện thoại tầm trung, tay ướt, ánh đèn phòng tắm: chữ ≥15px,
// vùng chạm ≥44px, điều hướng ở đáy màn hình vừa tầm ngón cái.
//
// KHÔNG có tiền ở đây. Lương tính theo cơ chế riêng ngoài phần mềm; màn này chỉ
// cho cô thấy việc đã làm và khách đánh giá thế nào.

import { useEffect, useMemo, useState } from 'react'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { useRouter } from '../router.jsx'
import PhotoUploader from '../components/PhotoUploader.jsx'
import { Alert, Empty, Progress, Spinner } from '../components/ui.jsx'
import {
	ROOM_TYPE_LABEL,
	SESSION_LABEL,
	dayKey,
	dayMonth,
	hour,
	minutes,
	shiftDay,
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
				<button className={`mob-tab${active === 'me' ? ' active' : ''}`} onClick={() => navigate('/cleaning/me')}>
					<span>📊</span>
					<span>Kết quả</span>
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
	const done = rows.filter((s) => s.status === 'submitted' || s.status === 'approved').length

	return (
		<CleanerShell active="today" title="Ca hôm nay">
			<div className="mob-sum">
				<div>
					<strong>{remaining}</strong>
					<span>phòng còn phải dọn</span>
				</div>
				<div>
					<strong>{done}</strong>
					<span>phòng đã xong</span>
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
						const finished = s.status === 'submitted' || s.status === 'approved'
						return (
							<button
								key={s.id}
								className={`mob-card${finished ? ' mob-card--done' : ''}`}
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
									{s.next_checkin_at ? ' · có khách vào ngay sau' : ''}
								</div>

								{s.guest_note && <div className="mob-note">Lưu ý: {s.guest_note}</div>}
								{s.late && <div className="mob-note mob-note--danger">Đã quá giờ khách vào</div>}

								<div className="mob-card-foot">
									<Progress percent={s.progress?.percent} />
									<span>
										{s.progress?.done_required}/{s.progress?.total_required} việc
									</span>
									{finished && !!s.minutes && <strong>{minutes(s.minutes)}</strong>}
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

	async function start() {
		try {
			const d = await api.startSession(sessionId)
			setSession(d.session)
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

			{!session.started_at && !submitted && (
				<button className="btn btn--primary btn--big" onClick={start}>
					Bắt đầu dọn
				</button>
			)}

			{submitted && (
				<div className="banner banner--ok">
					<strong>Đã đủ ảnh — ca này ghi nhận xong.</strong>
					<span>
						{session.minutes ? `Bạn dọn hết ${minutes(session.minutes)}. ` : ''}
						{session.status === 'approved' ? 'Quản lý đã duyệt.' : 'Đang chờ quản lý xem ảnh.'}
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

			<div className="mob-bottom">
				{p.complete ? (
					<div className="mob-bottom-ok">✅ Xong hết rồi</div>
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
		</CleanerShell>
	)
}

function isDone(session, item) {
	const st = session.items_state?.[item.id] || {}
	if (!item.require_photo) return !!st.checked
	return (st.photos || []).length >= (item.min_photos || 1)
}

// ─── Kết quả của tôi ──────────────────────────────────────────────────────

export function CleanerMePage() {
	const [day, setDay] = useState(dayKey())
	const [data, setData] = useState(null)
	const [reviews, setReviews] = useState(null)
	const [err, setErr] = useState('')

	useEffect(() => {
		setData(null)
		api
			.report({ day })
			.then(setData)
			.catch((e) => {
				setErr(e.message)
				setData({ rows: [], sessions: [] })
			})
	}, [day])

	useEffect(() => {
		api.reviews(14).then(setReviews).catch(() => setReviews({ stats: null }))
	}, [])

	const row = data?.rows?.[0]
	const today = dayKey()
	const st = reviews?.stats

	return (
		<CleanerShell active="me" title="Kết quả của tôi">
			<div className="mob-month">
				<button onClick={() => setDay(shiftDay(day, -1))} aria-label="Hôm trước">‹</button>
				<span>{day === today ? 'Hôm nay' : dayMonth(new Date(day).getTime())}</span>
				<button disabled={day >= today} onClick={() => setDay(shiftDay(day, 1))} aria-label="Hôm sau">›</button>
			</div>

			{data === null ? (
				<Spinner />
			) : (
				<div className="mob-sum mob-sum--3">
					<div>
						<strong>{row?.sessions || 0}</strong>
						<span>ca đã dọn</span>
					</div>
					<div>
						<strong>{row?.rooms || 0}</strong>
						<span>phòng</span>
					</div>
					<div>
						<strong>{minutes(row?.avg_minute)}</strong>
						<span>trung bình mỗi ca</span>
					</div>
				</div>
			)}

			{!!row?.late && (
				<div className="banner banner--warn">
					<strong>{row.late} ca xong sau giờ khách vào</strong>
					<span>Nếu do phòng quá bẩn hoặc thiếu đồ, báo quản lý để xếp thêm thời gian.</span>
				</div>
			)}

			<Alert>{err}</Alert>

			{/* Review là thước đo trực tiếp nhất việc dọn dẹp — cho cô thấy để tự
			    cải thiện, không phải để quản lý giữ riêng làm cơ sở khiển trách. */}
			<h2 className="mob-h2">Khách đánh giá (14 ngày qua)</h2>
			{reviews === null ? (
				<Spinner />
			) : !st || !st.total ? (
				<Empty icon="⭐">Chưa có đánh giá mới.</Empty>
			) : (
				<>
					<div className="mob-sum">
						<div>
							<strong>{st.avg_cleanliness ? st.avg_cleanliness.toFixed(1) : '—'}</strong>
							<span>điểm sạch sẽ trung bình</span>
						</div>
						<div>
							<strong>{st.low_clean}</strong>
							<span>lượt chê chưa sạch</span>
						</div>
					</div>

					{st.need_attention?.length > 0 && (
						<>
							<h2 className="mob-h2">Cần chú ý</h2>
							<div className="mob-list">
								{st.need_attention.slice(0, 5).map((r) => (
									<div key={r.id} className="rv rv--bad">
										<div className="rv-top">
											<span>{'⭐'.repeat(Math.max(1, r.cleanliness || r.overall))}</span>
											<span className="rv-room">{r.room_code}</span>
											<span className="rv-date">{dayMonth(r.created_at)}</span>
										</div>
										{r.comment && <div className="rv-text">{r.comment}</div>}
									</div>
								))}
							</div>
						</>
					)}

					<h2 className="mob-h2">Đánh giá gần đây</h2>
					<div className="mob-list">
						{st.recent.slice(0, 8).map((r) => (
							<div key={r.id} className={`rv${r.overall >= 4 ? ' rv--good' : ''}`}>
								<div className="rv-top">
									<span>{'⭐'.repeat(Math.max(1, r.overall))}</span>
									<span className="rv-room">{r.room_code}</span>
									<span className="rv-date">{dayMonth(r.created_at)}</span>
								</div>
								{r.comment && <div className="rv-text">{r.comment}</div>}
							</div>
						))}
					</div>
				</>
			)}

			<p className="help">
				Số liệu ở đây để bạn theo dõi công việc. Lương và thưởng tính theo cơ chế riêng, không nằm trong phần mềm này.
			</p>
		</CleanerShell>
	)
}
