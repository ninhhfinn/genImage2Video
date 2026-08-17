// Danh sách việc cần xử lý — một màn, ba vai nhìn khác nhau.
//
// Quản lý thấy tất cả và giao được việc; kỹ thuật thấy việc còn mở để tự nhận;
// cô dọn dẹp thấy vấn đề chính mình đã báo (để biết đã được xử lý chưa — báo
// xong rơi vào im lặng thì lần sau không ai buồn báo).
//
// Thứ tự do BACKEND sắp: quá hạn trước, rồi khẩn, rồi hạn gần nhất. Người mở
// danh sách ra là để biết phải làm gì trước, không phải đọc theo thứ tự báo.

import { useEffect, useMemo, useState } from 'react'
import { api, photoSrc } from './api.js'
import { useAuth } from './auth.jsx'
import { Alert, Empty, Spinner, Stat } from './components/ui.jsx'
import IssueForm from './components/IssueForm.jsx'
import { dayMonth, hour } from './format.js'

const STATUS_TONE = { open: 'warn', assigned: 'info', done: 'ok', rejected: 'neutral' }

/** Hạn còn bao lâu, nói theo kiểu người ta nói. */
function deadlineText(ms, overdue) {
	if (!ms) return 'không đặt hạn'
	const diff = ms - Date.now()
	const h = Math.round(Math.abs(diff) / 3600000)
	if (overdue) return h < 24 ? `quá hạn ${h} giờ` : `quá hạn ${Math.round(h / 24)} ngày`
	if (h < 1) return 'còn dưới 1 giờ'
	if (h < 24) return `còn ${h} giờ`
	return `còn ${Math.round(h / 24)} ngày`
}

export function IssueList({ issues, summary, role, staffs, onChanged, onError }) {
	const [openId, setOpenId] = useState('')

	async function act(fn) {
		try {
			await fn()
			onChanged?.()
		} catch (e) {
			onError?.(e.message)
		}
	}

	if (!issues.length) {
		return <Empty icon="✅">Không có việc nào trong mục này.</Empty>
	}

	return (
		<div className="issues">
			{issues.map((it) => {
				const open = openId === it.id
				return (
					<div key={it.id} className={`issue${it.overdue ? ' issue--overdue' : ''}${it.urgency === 'urgent' ? ' issue--urgent' : ''}`}>
						<button className="issue-head" onClick={() => setOpenId(open ? '' : it.id)}>
							<div className="issue-main">
								<div className="issue-title">
									{it.urgency === 'urgent' && <span className="issue-flag">KHẨN</span>}
									{it.category_text}
								</div>
								<div className="issue-room">
									{it.room_code} · {it.room_name}
									{it.facility_label ? ` · ${it.facility_label}` : ''}
								</div>
								<div className="issue-desc">{it.description}</div>
							</div>
							<div className="issue-side">
								<span className={`badge badge--${STATUS_TONE[it.status] || 'neutral'}`}>{it.status_text}</span>
								<span className={`issue-deadline${it.overdue ? ' danger' : ''}`}>
									{deadlineText(it.deadline_at, it.overdue)}
								</span>
								{it.assignee_name && <span className="meta">{it.assignee_name}</span>}
							</div>
						</button>

						{open && (
							<div className="issue-body">
								<div className="meta">
									Báo bởi {it.reporter_name || '—'} · {dayMonth(it.created_at)} {hour(it.created_at)}
									{it.deadline_at ? ` · hạn ${dayMonth(it.deadline_at)} ${hour(it.deadline_at)}` : ''}
								</div>

								{!!it.photos?.length && (
									<div className="strip">
										{it.photos.map((p, i) => (
											<img key={i} src={photoSrc(p.url)} alt="" onClick={() => window.open(photoSrc(p.url), '_blank')} />
										))}
									</div>
								)}

								{it.resolve_note && (
									<div className="issue-resolved">
										<strong>Đã xử lý:</strong> {it.resolve_note}
										{!!it.resolve_photos?.length && (
											<div className="strip">
												{it.resolve_photos.map((p, i) => (
													<img key={i} src={photoSrc(p.url)} alt="" />
												))}
											</div>
										)}
									</div>
								)}

								<div className="issue-actions">
									{role === 'handler' && it.status === 'open' && (
										<button className="btn btn--primary" onClick={() => act(() => api.claimIssue(it.id))}>
											Nhận việc này
										</button>
									)}

									{role === 'admin' && (
										<>
											<select
												value={it.assignee_id || ''}
												onChange={(e) => act(() => api.assignIssue({ id: it.id, assignee_id: e.target.value }))}
											>
												<option value="">— Chưa giao —</option>
												{staffs.map((s) => (
													<option key={s.id} value={s.id}>
														{s.name} {s.role === 'handler' ? '(kỹ thuật)' : ''}
													</option>
												))}
											</select>
											<input
												type="datetime-local"
												value={it.deadline_at ? new Date(it.deadline_at - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''}
												onChange={(e) =>
													e.target.value &&
													act(() => api.assignIssue({ id: it.id, deadline_at: new Date(e.target.value).getTime() }))
												}
											/>
											<button
												className="btn btn--ghost"
												onClick={() =>
													act(() =>
														api.assignIssue({ id: it.id, urgency: it.urgency === 'urgent' ? 'normal' : 'urgent' }),
													)
												}
											>
												{it.urgency === 'urgent' ? 'Bỏ đánh dấu khẩn' : 'Đánh dấu khẩn'}
											</button>
										</>
									)}

									{(role === 'admin' || it.assignee_id) && it.status !== 'done' && it.status !== 'rejected' && (
										<ResolveButton issue={it} role={role} onDone={onChanged} onError={onError} />
									)}

									{role === 'admin' && it.status !== 'rejected' && it.status !== 'done' && (
										<button
											className="btn btn--danger"
											onClick={() => act(() => api.resolveIssue({ id: it.id, status: 'rejected', note: 'Không cần xử lý' }))}
										>
											Bỏ qua
										</button>
									)}
								</div>
							</div>
						)}
					</div>
				)
			})}
		</div>
	)
}

function ResolveButton({ issue, role, onDone, onError }) {
	const [open, setOpen] = useState(false)
	const [note, setNote] = useState('')
	const [photos, setPhotos] = useState([])
	const [busy, setBusy] = useState(false)

	if (!open) {
		return (
			<button className="btn btn--primary" onClick={() => setOpen(true)}>
				Đánh dấu đã xong
			</button>
		)
	}
	return (
		<div className="resolve-box">
			<input placeholder="Đã xử lý thế nào?" value={note} onChange={(e) => setNote(e.target.value)} />
			<button
				className="btn btn--primary"
				disabled={busy}
				onClick={async () => {
					setBusy(true)
					try {
						await api.resolveIssue({ id: issue.id, status: 'done', note, photos })
						onDone?.()
					} catch (e) {
						onError?.(e.message)
					} finally {
						setBusy(false)
					}
				}}
			>
				{busy ? 'Đang lưu…' : 'Xác nhận xong'}
			</button>
			<button className="btn btn--ghost" onClick={() => setOpen(false)}>
				Huỷ
			</button>
		</div>
	)
}

/** Thẻ số liệu dùng chung cho báo cáo hằng ngày. */
export function IssueSummaryStats({ summary }) {
	const s = summary || {}
	return (
		<div className="stats">
			<Stat label="Chờ nhận" value={s.open || 0} tone={s.open ? 'warn' : 'ok'} sub="chưa ai phụ trách" />
			<Stat label="Quá hạn" value={s.overdue || 0} tone={s.overdue ? 'danger' : 'ok'} />
			<Stat label="Khẩn chưa xong" value={s.urgent || 0} tone={s.urgent ? 'danger' : 'ok'} />
			<Stat label="Đang xử lý" value={s.assigned || 0} />
			<Stat label="Báo mới hôm nay" value={s.new_today || 0} sub={`${s.done_today || 0} việc xong hôm nay`} />
		</div>
	)
}

/** Bộ lọc + danh sách, dùng cho cả màn quản lý lẫn màn kỹ thuật. */
export function IssuesBoard({ role, showReport = false }) {
	const [data, setData] = useState(null)
	const [staffs, setStaffs] = useState([])
	const [rooms, setRooms] = useState([])
	const [tab, setTab] = useState(role === 'handler' ? 'open' : 'all')
	const [err, setErr] = useState('')
	const [reporting, setReporting] = useState(false)

	async function load() {
		try {
			const q = new URLSearchParams()
			if (tab === 'open') q.set('open', '1')
			if (tab === 'mine') q.set('mine', '1')
			if (tab === 'urgent') q.set('urgency', 'urgent')
			if (tab === 'done') q.set('status', 'done')
			setData(await api.issues(q.toString()))
			setErr('')
		} catch (e) {
			setErr(e.message)
			setData({ issues: [], summary: {} })
		}
	}

	useEffect(() => {
		setData(null)
		load()
	}, [tab])

	useEffect(() => {
		if (role === 'admin') api.staffs().then((d) => setStaffs(d.staffs || [])).catch(() => {})
		api.rooms().then((d) => setRooms(d.rooms || [])).catch(() => {})
	}, [role])

	const issues = data?.issues || []
	// Quá hạn lên đầu để mắt bắt được ngay, dù backend đã sắp sẵn.
	const overdue = useMemo(() => issues.filter((i) => i.overdue).length, [issues])

	const TABS =
		role === 'handler'
			? [
					{ k: 'open', t: 'Việc cần làm' },
					{ k: 'mine', t: 'Việc của tôi' },
					{ k: 'done', t: 'Đã xong' },
				]
			: [
					{ k: 'all', t: 'Tất cả' },
					{ k: 'open', t: 'Chưa đóng' },
					{ k: 'urgent', t: 'Khẩn' },
					{ k: 'done', t: 'Đã xong' },
				]

	return (
		<>
			{showReport && <IssueSummaryStats summary={data?.summary} />}

			<div className="row-actions">
				{TABS.map((x) => (
					<button key={x.k} className={`chip${tab === x.k ? ' active' : ''}`} onClick={() => setTab(x.k)}>
						{x.t}
					</button>
				))}
				<button className="btn btn--ghost" onClick={load}>Tải lại</button>
				<button className="btn btn--primary" onClick={() => setReporting(true)}>+ Báo vấn đề</button>
				{!!overdue && <span className="meta meta--danger">{overdue} việc quá hạn</span>}
			</div>

			<Alert>{err}</Alert>

			{data === null ? (
				<Spinner />
			) : (
				<IssueList
					issues={issues}
					summary={data.summary}
					role={role}
					staffs={staffs}
					onChanged={load}
					onError={setErr}
				/>
			)}

			{reporting && (
				<div className="modal" onClick={() => setReporting(false)}>
					<div className="modal-in modal-in--wide" onClick={(e) => e.stopPropagation()}>
						<h2>Báo vấn đề</h2>
						<IssueForm
							rooms={rooms}
							onCancel={() => setReporting(false)}
							onDone={() => {
								setReporting(false)
								load()
							}}
						/>
					</div>
				</div>
			)}
		</>
	)
}

/** Màn riêng cho nhân sự kỹ thuật — họ không thấy ca dọn hay doanh thu. */
export function HandlerPage() {
	const { user, logout } = useAuth()
	return (
		<div className="handler">
			<header className="mob-top">
				<span className="mob-btn mob-btn--hidden" />
				<div className="mob-title">
					<span>Việc cần xử lý</span>
					<small>{user?.name}</small>
				</div>
				<button className="mob-btn" onClick={logout} aria-label="Đăng xuất">⎋</button>
			</header>
			<main className="handler-body">
				<IssuesBoard role="handler" showReport />
			</main>
		</div>
	)
}
