import React, { useContext } from 'react';
import { Button, Result } from 'antd';
import { useLocation, useNavigate } from 'react-router-dom';
import { UserContext } from '../App';

export default function AuthGate({ children, roles }) {
  const { user } = useContext(UserContext);
  const navigate = useNavigate();
  const location = useLocation();

  if (!user) {
    return <Result status="403" title="需要登录" subTitle="登录企业账号后可继续使用此功能。" extra={<Button type="primary" onClick={() => navigate('/login', { state: { from: location.pathname } })}>前往登录</Button>} />;
  }
  if (roles?.length && !roles.includes(user.role)) {
    return <Result status="403" title="无访问权限" subTitle="当前账号没有访问此工作区的角色权限。" />;
  }
  return children;
}
