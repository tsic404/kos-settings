# kos-settings

KOS 系统应用项目：设置与成员扩展包 — 集群与节点设置、维护窗口、成员与角色入口。

- 设计：`tsic/kos-design` 仓的 `kos-settings/`（`01-scope.md`、`02-contracts.md` SETTINGS-C-36、`03-acceptance.md`）
- 设计依据：`tsic/kos-design@05bf60a`
- 交付物：经 kos-sdk 打包的扩展包（KOSD-C-17、KOSD-C-18）；本产品是系统应用扩展，不是独立进程（KOSD ADR-033）

## 边界

- 设置项由扩展贡献，内核不硬编码（KOSD-C-23）；本仓的 `pkg/kossettings` 是这份声明的唯一来源。
- 不提供未声明或未安装贡献的项（KOSD-F-16）：页面与设置项随系统组件出现与消失。
- 不下发 kubeconfig 或服务账号令牌（KOSD-F-9）；产品只经控制面 API 读写，不持有集群凭据。
- 用户页与资源占用的采样四态由后续子单交付；维护窗口的项、入口与读写用例（`SETTINGS-TC-136`）在本仓交付，窗口外拒绝的边界由 kosd 的维护发起按 KOSD-C-65 判窗（`SETTINGS-TC-137`，用例落在 `tsic404/kosd`）。

## 包布局

| 路径 | 内容 | 契约 |
|---|---|---|
| `pkg/kossettings/declaration.go` | 设置项与设置页声明：集群组与节点组的项、来源类型、动作与不可修改原因 | SETTINGS-C-36、KOSD-C-18、KOSD-C-23 |
| `pkg/kossettings/readings.go` | 设置页的读模型：按声明的来源类型读取当前值，容量与用量分开 | SETTINGS-C-36、SETTINGS-C-71、KOSD-C-53 |
| `assets/settings/` | 设置页前端资源：逐项渲染当前值、来源类型、入口与不可修改原因；维护入口只对 Owner 出现 | SETTINGS-C-36、KOSD-C-17、KOSD-C-59、KOSD-C-73 |
| `pkg/kossettings/manifest.go` | 同一份声明生成的 kos-sdk 清单：KOSD-C-18 的封闭字段集合 | SDK-C-3 |
| `pkg/kossettings/package.go` | 经 kos-sdk 打包：清单、UI 资产与包摘要 | SDK-C-4、KOSD-C-17 |
| `cmd/kos-settings-pack/` | 打包入口：写出扩展包与摘要 | KOSD-C-17 |
| `tests/acceptance/SETTINGS-TC-62/` | 设置页两组的项、来源类型与修改入口 | SETTINGS-AC-31、SETTINGS-TC-62 |
| `tests/acceptance/SETTINGS-TC-138/` | 来源类型唯一：对象源随对象、数据库源随数据库、不可用给原因 | SETTINGS-AC-31、SETTINGS-TC-138 |
| `tests/acceptance/KOSD-TC-155/` | 单独装载、卸载与重装；包结构与摘要；其余产品与内核对象不受影响 | KOSD-AC-72、KOSD-TC-155 |

设置页是服务端数据投影（KOSD-C-22、KOSD-C-53）：页面外壳按声明的来源类型读取每一项，本仓提供声明、读模型与页面资源。
前端资源位于 `assets/settings/`，按 `assets/<path>` 入包并计入摘要：`index.html` 是入口文档，`app.js` 读取页面投影并提交声明的写入，`render.js` 逐项渲染当前值、来源类型、入口与不可修改原因，`settings.css` 提供样式。
入口按投影裁剪：Owner 的投影带维护窗口写入与维护发起/恢复入口，Member 的投影不带任何 action 或 entry，页面因此只读且无入口（KOSD-C-59）。
用户页（SETTINGS-C-1）仍由后续子单交付。

## 构建与测试

```sh
make lint        # gofmt 检查 + go vet + 前端模块语法检查
make test        # 单元与验收用例（Go 与 Node）
make acceptance  # 只跑 tests/acceptance/（Go 与 Node），-v
make pack        # 写出 dist/kos-settings.tar 与 dist/kos-settings.tar.digest
```

`make pack` 读 `assets/`（缺失即无 UI 资产）、校验声明、生成包与 `sha256:<hex>` 摘要；同一清单与资产总是产出同一结构与同一摘要。

## 声明要点

- 设置贡献一项：集群级维护窗口（集群作用域、用户写者、可修改），写入口为 `/api/v1/settings/cluster-maintenance-window`（SETTINGS-C-72）。
- 维护窗口的值是一周内允许发起维护的时段，写作 `<周几> HH:MM-HH:MM`（UTC），多个时段以逗号分隔，例如 `Sun 02:00-04:00,Wed 01:00-02:30`；未写入时值为空，维护发起不设时间限制。
- 设置页 20 项：集群组 9 项、节点组 11 项，逐项一个来源类型（object / computed / database / metric / read）。
- 可分配容量与实测用量是两项：容量读节点对象，用量读指标源；用量仅在样本覆盖全部节点时给出，其余状态标为不可用并给出原因（SETTINGS-C-71）。
- 节点级配置（资源预留等）不在对象模型内，列为不可修改项并给出原因。
- 维护发起与恢复入口只对 Owner 出现（KOSD-C-59）。
