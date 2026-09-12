import { useEffect, useState } from "react";

export const navigation = [
  {
    label: "工作空间",
    pages: [
      {
        id: "overview",
        label: "网络总览",
        icon: "◎",
        description: "查看节点与房间的实时连接，及时发现网络异常。",
      },
      {
        id: "nodes",
        label: "节点管理",
        icon: "◈",
        description: "登记基础设施节点，管理配置与运行状态。",
      },
      {
        id: "rooms",
        label: "房间与成员",
        icon: "◫",
        description: "查看房间网络、成员授权与连接质量。",
      },
      {
        id: "users",
        label: "用户管理",
        icon: "◉",
        description: "查询账号、设备与会话，处理用户访问权限。",
      },
      {
        id: "games",
        label: "游戏管理",
        icon: "◇",
        description: "维护游戏目录与同房成员允许使用的网络规则。",
      },
    ],
  },
  {
    label: "发布中心",
    pages: [
      {
        id: "releases",
        label: "版本与安装包",
        icon: "▣",
        description: "导入签名清单、验证安装包并管理版本发布。",
      },
      {
        id: "devices",
        label: "设备版本",
        icon: "▤",
        description: "查看版本分布、设备上报与实际安装结果。",
      },
    ],
  },
  {
    label: "系统设置",
    pages: [
      {
        id: "oidc",
        label: "账号登录",
        icon: "⌘",
        description: "设置身份提供方，开放客户端账号登录与访客绑定。",
      },
      {
        id: "sources",
        label: "更新存储源",
        icon: "▱",
        description: "配置安装包的存储、下载地址与备用源优先级。",
      },
      {
        id: "policies",
        label: "更新规则",
        icon: "≋",
        description: "按平台设置推荐版本与最低允许版本。",
      },
      {
        id: "password",
        label: "管理员密码",
        icon: "⌑",
        description: "修改当前管理员密码；保存后需要重新登录。",
      },
    ],
  },
  {
    label: "运行信息",
    pages: [
      {
        id: "deployment",
        label: "部署信息",
        icon: "⊞",
        description: "查看部署参数、证书有效期与监控数据来源。",
      },
      {
        id: "events",
        label: "操作记录",
        icon: "≡",
        description: "追踪节点操作结果，查询管理员审计记录。",
      },
    ],
  },
] as const;

type Page = (typeof navigation)[number]["pages"][number];
export type PageID = Page["id"];
export const pages = navigation.flatMap<Page>((group) => group.pages);
function currentPage(): PageID {
  const id = window.location.hash.slice(1);
  return pages.find((page) => page.id === id)?.id || "overview";
}

export function useAdminPage() {
  const [page, setPage] = useState(currentPage);
  useEffect(() => {
    const restore = () => setPage(currentPage());
    window.addEventListener("hashchange", restore);
    return () => window.removeEventListener("hashchange", restore);
  }, []);
  return page;
}

export function Navigation({ current }: { current: PageID }) {
  return (
    <nav className="main-navigation" aria-label="主导航">
      {navigation.map((group) => (
        <div className="nav-group" key={group.label}>
          <p className="eyebrow">{group.label}</p>
          {group.pages.map((page) => (
            <a
              key={page.id}
              href={`#${page.id}`}
              className={current === page.id ? "selected" : ""}
              aria-current={current === page.id ? "page" : undefined}
            >
              <span aria-hidden="true">{page.icon}</span>
              {page.label}
            </a>
          ))}
        </div>
      ))}
    </nav>
  );
}
