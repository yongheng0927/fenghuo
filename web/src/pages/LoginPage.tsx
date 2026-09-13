import { useEffect, useState } from 'react'
import { Alert, Button, Card, Divider, Form, Input, Typography, theme } from 'antd'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom'

import { useAuth } from '../auth/useAuth'
import { getSetupStatus, getSystemInfo } from '../api'
import { errorMessage } from '../api/client'
import { assetUrl, runtimeConfig } from '../config'
import type { SystemInfo } from '../api/types'

export default function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [sysInfo, setSysInfo] = useState<SystemInfo | null>(null)
  // 页面底色跟随明暗主题（深色模式下硬编码浅色会露出亮色背景）
  const {
    token: { colorBgLayout },
  } = theme.useToken()

  useEffect(() => {
    // system/info 是公开接口（M4）：登录前用它决定是否展示 OAuth 按钮
    getSystemInfo()
      .then(setSysInfo)
      .catch(() => undefined)
  }, [])

  useEffect(() => {
    // 未完成首次初始化时引导去创建管理员；请求失败静默留在登录页
    getSetupStatus()
      .then((status) => {
        if (!status.initialized) navigate('/setup', { replace: true })
      })
      .catch(() => undefined)
  }, [navigate])

  useEffect(() => {
    // OAuth 失败时后端会带 ?oauth_error=... 重定向回本页
    const oauthError = searchParams.get('oauth_error')
    if (oauthError) setError(oauthError)
  }, [searchParams])

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true)
    setError('')
    try {
      await login(values.username, values.password)
      const from = (location.state as { from?: string } | null)?.from
      navigate(from && from !== '/login' ? from : '/dashboard', { replace: true })
    } catch (err) {
      setError(errorMessage(err, '登录失败，请稍后重试'))
    } finally {
      setLoading(false)
    }
  }

  const oauthEnabled = sysInfo?.oauth_enabled ?? runtimeConfig.oauthEnabled

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: colorBgLayout,
      }}
    >
      <Card style={{ width: 400, boxShadow: '0 4px 16px rgba(0,0,0,0.08)' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <img src={assetUrl('logo.svg')} alt="烽火台" style={{ width: 56, height: 56, marginBottom: 8 }} />
          <Typography.Title level={3} style={{ marginBottom: 4 }}>
            烽火台
          </Typography.Title>
          <Typography.Text type="secondary">
            自托管告警转发中台 · Alertmanager → 飞书 / 钉钉 / 企业微信
          </Typography.Text>
        </div>
        {error && <Alert type="error" message={error} showIcon style={{ marginBottom: 16 }} />}
        <Form onFinish={onFinish} size="large">
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" autoComplete="current-password" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Button type="primary" htmlType="submit" block loading={loading}>
              登录
            </Button>
          </Form.Item>
        </Form>
        {oauthEnabled && (
          <>
            <Divider plain style={{ color: '#999', fontSize: 12 }}>
              或
            </Divider>
            {/* M4: 飞书 OAuth 登录入口（后端 GET /api/auth/oauth/feishu） */}
            <Button block href={`${runtimeConfig.base}api/auth/oauth/feishu`.replace(/\/{2,}/g, '/')}>
              使用飞书账号登录
            </Button>          </>
        )}
      </Card>
    </div>
  )
}
