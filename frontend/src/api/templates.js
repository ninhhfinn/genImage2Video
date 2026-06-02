import axios from 'axios'

const api = axios.create({ baseURL: '/api', timeout: 10000 })

export async function fetchTemplates() {
  const res = await api.get('/templates')
  return res.data || []
}
