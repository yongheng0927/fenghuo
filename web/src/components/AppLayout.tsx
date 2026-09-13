import { useEffect, useState } from 'react'
import { Layout, Menu, Dropdown, Avatar, Tag, Typography, message, theme } from 'antd'
import {
  AlertOutlined,
  ApiOutlined,
  DashboardOutlined,
  FileTextOutlined,
  LogoutOutlined,
  NodeIndexOutlined,
  SendOutlined,
  SettingOutlined,
  SwapOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'

import { assetUrl, runtimeConfig } from '../config'
import { useAuth } from '../auth/useAuth'

const { Sider, Header, Content } = Layout

const ROLE_LABEL: Record<string, { text: string; color: string }> = {
  viewer: { text: 'Viewer', color: 'default' },
  editor: { text: 'Editor', color: 'blue' },
  admin: { text: 'Admin', color: 'gold' },
}

export default function AppLayout() {
  const { user, canWrite, isAdmin, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  // 侧栏折叠态（图标模式）：折叠时顶部只显示项目图标，不显示名称
  const [collapsed, setCollapsed] = useState(false)
  const {
    token: { colorBgContainer },
  } = theme.useToken()

  const items = [
    { key: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
    { key: '/alerts', icon: <AlertOutlined />, label: '告警记录' },
    ...(canWrite ? [{ key: '/deliveries', icon: <SendOutlined />, label: '失败投递' }] : []),
    { type: 'divider' as const },
    { key: '/sources', icon: <ApiOutlined />, label: '接入源' },
    { key: '/channels', icon: <NodeIndexOutlined />, label: '通知渠道' },
    { key: '/templates', icon: <FileTextOutlined />, label: '消息模板' },
    { key: '/rules', icon: <SwapOutlined />, label: '路由规则' },
    { type: 'divider' as const },
    { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
    ...(isAdmin ? [{ key: '/users', icon: <TeamOutlined />, label: '用户管理' }] : []),
  ]

  const selectedKey =
    items
      .filter((it) => 'key' in it)
      .map((it) => (it as { key: string }).key)
      .find((k) => location.pathname.startsWith(k)) ?? '/dashboard'

  // 按当前路由更新浏览器标签页标题
  const selectedLabel = (
    items.find((it) => 'key' in it && (it as { key: string }).key === selectedKey) as
      | { label?: string }
      | undefined
  )?.label
  useEffect(() => {
    document.title = selectedLabel ? `${selectedLabel} · 烽火台` : '烽火台'
  }, [selectedLabel])

  // 只有 admin 可见角色标识，其余角色不向本人展示
  const role = user && isAdmin ? ROLE_LABEL[user.role] : undefined

  const onLogout = async () => {
    await logout()
    message.success('已退出登录')
    navigate('/login', { replace: true })
  }

  return (
    // 高度锁死在视口：滚动只发生在 Content 内部，侧栏（logo/菜单）与
    // Header 固定不随页面滚动
    <Layout style={{ height: '100vh' }}>
      <Sider theme="dark" width={208} breakpoint="lg" collapsible collapsed={collapsed} onCollapse={setCollapsed}>
        <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          <div
            style={{
              height: 56,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 8,
              color: '#fff',
              fontSize: 17,
              fontWeight: 600,
              letterSpacing: 0.5,
            }}
          >
            <img src={assetUrl('logo.svg')} alt="烽火台" style={{ width: 28, height: 28 }} />
            {!collapsed && <span>烽火台</span>}
          </div>
          <Menu
            theme="dark"
            mode="inline"
            selectedKeys={[selectedKey]}
            items={items}
            onClick={({ key }) => navigate(key)}
          />
          {runtimeConfig.version && (
            <div
              style={{
                marginTop: 'auto',
                padding: '12px 24px',
                color: 'rgba(255,255,255,0.45)',
                fontSize: 12,
              }}
            >
              v{runtimeConfig.version}
            </div>
          )}
        </div>
      </Sider>
      <Layout style={{ overflow: 'hidden' }}>
        <Header
          style={{
            background: colorBgContainer,
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            borderBottom: '1px solid #f0f0f0',
          }}
        >
          <Dropdown
            menu={{
              items: [
                { key: 'settings', icon: <SettingOutlined />, label: '系统设置' },
                { type: 'divider' },
                { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
              ],
              onClick: ({ key }) => {
                if (key === 'logout') void onLogout()
                if (key === 'settings') navigate('/settings')
              },
            }}
          >
            <div style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8 }}>
              <Avatar src={user?.avatar_url || undefined} icon={<UserOutlined />} size="small" />
              <Typography.Text strong>{user?.name || user?.username}</Typography.Text>
              {role && <Tag color={role.color} style={{ marginInlineEnd: 0 }}>{role.text}</Tag>}
            </div>
          </Dropdown>
        </Header>
        <Content style={{ padding: 24, overflow: 'auto' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
