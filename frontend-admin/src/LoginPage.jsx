import { useState } from 'react'
import { api } from './api.js'
import { useAuth } from './auth.jsx'
import { Link, useRouter } from './router.jsx'
import { Alert } from './components/ui.jsx'

export function LoginPage() {
	const { login } = useAuth()
	const { navigate } = useRouter()
	const [phone, setPhone] = useState('')
	const [password, setPassword] = useState('')
	const [show, setShow] = useState(false)
	const [busy, setBusy] = useState(false)
	const [err, setErr] = useState('')

	async function submit(e) {
		e.preventDefault()
		if (busy) return
		setErr('')
		if (!phone.trim()) return setErr('Bạn chưa nhập số điện thoại.')
		if (!password) return setErr('Bạn chưa nhập mật khẩu.')
		setBusy(true)
		try {
			const user = await login(phone.trim(), password)
			navigate(user.role === 'admin' ? '/' : '/cleaning', { replace: true })
		} catch (e2) {
			setErr(e2.message)
		} finally {
			setBusy(false)
		}
	}

	return (
		<div className="auth">
			<div className="auth-box">
				<div className="auth-brand">
					<div className="auth-mark">🧹</div>
					<div>
						<strong>Dọn dẹp Dayladau</strong>
						<small>Checklist &amp; chấm công</small>
					</div>
				</div>

				<h1>Đăng nhập</h1>

				<form onSubmit={submit} className="form">
					<label className="field">
						<span>Số điện thoại</span>
						<input
							type="tel"
							inputMode="numeric"
							autoComplete="username"
							placeholder="VD: 0912345678"
							value={phone}
							onChange={(e) => setPhone(e.target.value)}
						/>
					</label>

					<label className="field">
						<span>Mật khẩu</span>
						<div className="field-row">
							<input
								type={show ? 'text' : 'password'}
								autoComplete="current-password"
								placeholder="Nhập mật khẩu"
								value={password}
								onChange={(e) => setPassword(e.target.value)}
							/>
							{/* Nút hiện mật khẩu không phải trang trí: gõ trên bàn phím điện
							    thoại bằng tay ướt sai luôn là chuyện thường, và thử lại mà
							    không thấy mình gõ gì là bỏ cuộc. */}
							<button type="button" className="btn btn--ghost" onClick={() => setShow((v) => !v)}>
								{show ? 'Ẩn' : 'Hiện'}
							</button>
						</div>
					</label>

					<Alert>{err}</Alert>

					<button type="submit" className="btn btn--primary btn--big" disabled={busy}>
						{busy ? 'Đang đăng nhập…' : 'Đăng nhập'}
					</button>

					<Link to="/register" className="link-center">
						Chưa có tài khoản? Đăng ký ở đây
					</Link>

					<p className="help">
						Đăng ký xong cần quản lý duyệt mới đăng nhập được. Chờ lâu thì gọi cho quản lý của bạn nhé.
					</p>
				</form>
			</div>
		</div>
	)
}

export function RegisterPage() {
	const { navigate } = useRouter()
	const [form, setForm] = useState({ name: '', phone: '', password: '', confirm: '', zone: '', note: '' })
	const [busy, setBusy] = useState(false)
	const [err, setErr] = useState('')
	const [done, setDone] = useState(false)

	const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

	async function submit(e) {
		e.preventDefault()
		if (busy) return
		setErr('')
		if (!form.name.trim()) return setErr('Bạn chưa nhập họ tên.')
		if (!/^0\d{8,10}$/.test(form.phone.replace(/\s/g, ''))) {
			return setErr('Số điện thoại chưa đúng. Ví dụ: 0912345678')
		}
		if (form.password.length < 6) return setErr('Mật khẩu cần ít nhất 6 ký tự.')
		if (form.password !== form.confirm) return setErr('Hai lần nhập mật khẩu chưa khớp nhau.')

		setBusy(true)
		try {
			await api.register({
				name: form.name.trim(),
				phone: form.phone.replace(/\s/g, ''),
				password: form.password,
				zone: form.zone.trim(),
				note: form.note.trim(),
			})
			setDone(true)
		} catch (e2) {
			setErr(e2.message)
		} finally {
			setBusy(false)
		}
	}

	if (done) {
		return (
			<div className="auth">
				<div className="auth-box auth-box--center">
					<div className="done-ico">✅</div>
					<h1>Đã gửi đăng ký</h1>
					<p>
						Đăng ký của bạn đã gửi tới quản lý. Khi được duyệt, bạn đăng nhập bằng số điện thoại và mật khẩu vừa tạo.
					</p>
					<button className="btn btn--primary btn--big" onClick={() => navigate('/login', { replace: true })}>
						Về trang đăng nhập
					</button>
				</div>
			</div>
		)
	}

	return (
		<div className="auth">
			<div className="auth-box">
				<h1>Đăng ký làm dọn dẹp</h1>
				<form onSubmit={submit} className="form">
					<label className="field">
						<span>Họ và tên</span>
						<input placeholder="VD: Nguyễn Thị Lan" value={form.name} onChange={set('name')} />
					</label>
					<label className="field">
						<span>Số điện thoại</span>
						<input type="tel" inputMode="numeric" placeholder="VD: 0912345678" value={form.phone} onChange={set('phone')} />
					</label>
					<label className="field">
						<span>Khu vực bạn nhận ca</span>
						<input placeholder="VD: Cầu Giấy" value={form.zone} onChange={set('zone')} />
					</label>
					<label className="field">
						<span>Mật khẩu (ít nhất 6 ký tự)</span>
						<input type="password" value={form.password} onChange={set('password')} />
					</label>
					<label className="field">
						<span>Nhập lại mật khẩu</span>
						<input type="password" value={form.confirm} onChange={set('confirm')} />
					</label>
					<label className="field">
						<span>Ghi chú cho quản lý (không bắt buộc)</span>
						<input placeholder="VD: chị Mai giới thiệu, làm được ca sáng" value={form.note} onChange={set('note')} />
					</label>

					<Alert>{err}</Alert>

					<button type="submit" className="btn btn--primary btn--big" disabled={busy}>
						{busy ? 'Đang gửi…' : 'Gửi đăng ký'}
					</button>
					<Link to="/login" className="link-center">
						Đã có tài khoản? Đăng nhập
					</Link>
				</form>
			</div>
		</div>
	)
}
