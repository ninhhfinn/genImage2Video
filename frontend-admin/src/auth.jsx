// Phiên đăng nhập dùng chung.
//
// Một hệ tài khoản cho cả hai vai — `user.role` quyết định thấy màn nào. Backend
// vẫn kiểm quyền độc lập ở mọi endpoint; chỗ này chỉ để không hiển thị nút mà
// bấm vào sẽ bị từ chối.

import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import { api, getToken, setToken } from './api.js'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
	const [user, setUser] = useState(null)
	const [loading, setLoading] = useState(true)

	useEffect(() => {
		if (!getToken()) {
			setLoading(false)
			return
		}
		api
			.me()
			.then((d) => setUser(d.user))
			// Token cũ/hết hạn: coi như chưa đăng nhập, không hiện lỗi đỏ — người dùng
			// mở app sau một tháng không cần đọc thông báo lỗi, chỉ cần màn đăng nhập.
			.catch(() => setToken(''))
			.finally(() => setLoading(false))
	}, [])

	const login = useCallback(async (phone, password) => {
		const d = await api.login(phone, password)
		setToken(d.token)
		setUser(d.user)
		return d.user
	}, [])

	const logout = useCallback(async () => {
		try {
			await api.logout()
		} catch {
			/* mất mạng thì vẫn phải đăng xuất được ở phía máy */
		}
		setToken('')
		setUser(null)
	}, [])

	return <AuthContext.Provider value={{ user, loading, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth() {
	return useContext(AuthContext)
}

export const isAdmin = (user) => !!user && user.role === 'admin'
