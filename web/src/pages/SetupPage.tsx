import { useEffect, useState } from 'react'
import { Alert, Button, Card, Form, Input, Typography, theme } from 'antd'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'

import { useAuth } from '../auth/useAuth'
import { getSetupStatus, setup } from '../api'
import { errorMessage } from '../api/client'
import { assetUrl } from '../config'

export default function SetupPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  // 页面底色跟随明暗主题（同 LoginPage）
  const {
    token: { colorBgLayout },
  } = theme.useToken()

  useEffect(() => {
    // 已完成初始化时不应停留在本页；请求失败则静默留在设置页
    getSetupStatus()
      .then((status) => {
        if (status.initialized) navigate('/login', { replace: true })
      })
      .catch(() => undefined)
  }, [navigate])

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true)
    setError('')
    try {
      await setup(values.username, values.password)
      // setup 成功即账号已创建，复用登录流程写入 token 和用户信息
      await login(values.username, values.password)
      navigate('/dashboard', { replace: true })
    } catch (err) {
      setError(errorMessage(err, '初始化失败，请稍后重试'))
    } finally {
      setLoading(false)
    }
  }

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
            初始化烽火台
          </Typography.Title>
          <Typography.Text type="secondary">首次启动，请创建管理员账号</Typography.Text>
        </div>
        {error && <Alert type="error" message={error} showIcon style={{ marginBottom: 16 }} />}
        <Form onFinish={onFinish} size="large">
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="管理员用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 8, message: '密码至少 8 位' },
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码（至少 8 位）" autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="confirm"
            dependencies={['password']}
            rules={[
              { required: true, message: '请再次输入密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) return Promise.resolve()
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="确认密码" autoComplete="new-password" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Button type="primary" htmlType="submit" block loading={loading}>
              创建并登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}
