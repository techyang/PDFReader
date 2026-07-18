# 批量打印设计

**Goal:** 给阅读器加打印能力——一个统一的「打印」对话框，既能打印当前打开的单个文件，也能批量选择多个 PDF 文件、共用一份打印设置依次打印。

**背景：** 原设计文档（`docs/superpowers/specs/2026-07-11-pdf-reader-design.md`）把"打印"明确列在"范围之外（本期不做）"。这是全部 21 个任务完成后追加的新功能，走独立的 brainstorming → 设计 → 实现流程，不回头改原计划。当前代码库里没有任何打印相关代码；`github.com/klippa-app/go-pdfium` 未包出打印 API，`github.com/lxn/walk` 没有任何 Printer/PrintDialog 封装，只有 `github.com/lxn/win` 提供了原始的 GDI 打印调用（`CreateDC`/`StartDoc`/`StartPage`/`EndPage`/`EndDoc`/`DEVMODE`/`DOCINFO`/`EnumPrinters`/`GetDefaultPrinter`/`DocumentProperties`/`DeviceCapabilities`）。

参考截图：PDFgear 的打印对话框（单文件设置面板：打印机/份数/灰度/范围/双面/纸张大小/方向/页面缩放）与批量打印界面（左侧文件列表 + 同一套设置面板 + 预览）。

---

## 1. 入口与整体架构

- 「文件」菜单新增「打印...(&P)」，快捷键 `Ctrl+P`。不新增工具栏按钮。
- 只做**一个**对话框，不像 PDFgear 那样分单文件/批量两层：对话框左侧永远是文件列表，右侧永远是共享设置面板。列表只有一个文件时，效果就是单文件打印；列表为空时禁用「打印」按钮。
- 打开对话框时，若当前有激活的标签页，自动把它加入左侧列表，且**复用该标签页已经打开（可能已解密）的 `pdfengine.Document`**，不重新弹密码框、不重新用 `a.pool.Open` 打开一份新的。没有打开文件时列表为空。
- 新增两个包：
  - `internal/print`：不依赖 `walk`，纯 Go + `lxn/win` 系统调用。负责打印机枚举、`DEVMODE` 构造、页码范围字符串解析、GDI 打印管线本身。
  - `internal/ui/printdialog.go`：对话框 UI（声明式 + 少量命令式），调用 `internal/print` 的能力，风格与现有 `internal/ui/searchbar.go`、`internal/ui/app.go` 里的对话框代码一致。

## 2. 打印对话框 UI

### 左侧：文件列表

- 顶部「+ 添加PDF文件」按钮 → `walk.FileDialog`（多选；若 walk 的 `FileDialog` 不支持多选，实现阶段改为连续多次弹出单选，不影响这里的设计）。
- 每行：通用 PDF 图标 + 文件名 + 页数（如「report.pdf　12 页」），不渲染真实缩略图。
- 右键菜单「移除」：把该项从列表中去掉；若该项是当场为打印临时打开的文档（见下），移除时一并 `Close()`。
- 新添加的文件，若不是已经打开的那个标签页对应的文件，用 `a.pool.Open` 单独打开一份，只用于读取页数和渲染，不出现在标签页栏里。加密文档正常弹密码框；用户取消或密码错误，则该项不加入列表（不是加入后标红——保持列表里的项都是"确认可打印"的），弹一次性的 `MsgBox` 说明原因。
- 列表项数据结构（`internal/ui` 内部）：
  ```go
  type printItem struct {
      path      string
      doc       *pdfengine.Document
      ownsDoc   bool   // true：对话框自己 Open 的，Close 由对话框负责；false：借用某个标签页的，不能 Close
      pageCount int
      rangeSpec string // 该文件自己的页码范围输入，默认 ""（=所有页）
  }
  ```

### 右侧：设置面板（除"范围"外，所有文件共享同一份）

- 打印机下拉：`win.EnumPrinters` 枚举本机打印机名，默认选中 `config.Config.LastPrinter`；取不到（从未打印过/该打印机已不存在）则退回 `win.GetDefaultPrinter`。
- 「属性」按钮：调用 `win.DocumentProperties` 弹出打印机驱动自带的原生设置窗口，返回的 `DEVMODE` 覆盖当前面板里能表达的份数/双面/纸张/方向等字段的默认值（驱动的设置优先，用户之后仍可在我们的面板里再改）。
- 份数：`walk.NumberEdit`，写入 `DEVMODE.dmCopies`；多份交给驱动/后台打印机处理，不在应用层手工重复整份文档。
- 灰度打印：`walk.CheckBox`。同时设置 `DEVMODE.dmColor = DMCOLOR_MONOCHROME`（部分驱动可能忽略）**和**在渲染完每页位图后手工做灰度转换（RGB 加权平均），确保视觉效果始终正确，不依赖驱动是否遵守 `dmColor`。
- 双面打印：`walk.CheckBox`，对应 `DMDUP_VERTICAL`/`DMDUP_SIMPLEX`（不做长边/短边细分）。
- 范围：`所有页`/`选择页`两个 `RadioButton` + 一个文本框（`"1,8,9-12"` 语法）。**这是唯一按文件单独存储的设置**：左侧列表选中哪个文件，这个控件组读写的就是该 `printItem.rangeSpec`；切换选中项时同步刷新显示。默认「所有页」（`rangeSpec == ""`）。
- 纸张大小下拉：打印机变化时用 `win.DeviceCapabilities`（`DC_PAPERNAMES`/`DC_PAPERS`）查询该打印机支持的纸张列表填充；查询失败（返回负数/驱动不支持）时退化为固定的 `A4`/`Letter`/`Legal` 三项。
- 纸张方向：`纵向`/`横向` 单选，对应 `DEVMODE.dmOrientation`。
- 页面大小调整：`适合页面`/`实际大小`/`页面缩放(百分比)` 三选一，决定每页渲染位图贴到纸张上时的缩放算法（见下节公式）。
- 不做「打印注释」——本阅读器没有批注功能，此项不适用。
- 不做实时预览面板——先做最小可用版本，缺失时用户可以先在主窗口翻阅确认内容。
- 底部「取消」「打印」两个按钮；文件列表为空或没有选中打印机时「打印」禁用。

## 3. 打印管线（`internal/print/job.go`）

固定用 **300 DPI** 渲染每页（不查询打印机实际 DPI），再用 `StretchDIBits` 缩放贴到纸张——300 DPI 对文本/图表类 PDF 已经足够清晰，内存和耗时可预测，不会因为遇到 600/1200 DPI 的激光打印机产生过大位图。

```
for each file in 文件列表:
    if 用户已取消: 停止，不再处理后续文件
    devMode := 由共享设置构造（份数/双面/纸张/方向/灰度提示）
    hdc := CreateDC("WINSPOOL", 打印机名, nil, devMode)
    if CreateDC 失败: 记入失败列表，跳过，继续下一个文件
    StartDoc(hdc, &DOCINFO{lpszDocName: 文件名})
    pages := parseRange(该文件.rangeSpec, 该文件.pageCount)  // ""→ 全部页
    for each 页码 in pages:
        StartPage(hdc)
        img, err := doc.RenderPage(页码, 300)
        if err != nil: EndPage，中止该文件的后续页码（不影响批次里其它文件）
        if 灰度: img = toGrayscale(img)
        目标矩形 := 按纸张尺寸(减边距) + 缩放模式(适合页面/实际大小/百分比) 计算
        StretchDIBits(hdc, img, 目标矩形)
        EndPage(hdc)
        progressCB(文件序号, 文件总数, 文件名, 页码, 该文件总页数) 返回 true 则请求取消
    EndDoc(hdc); DeleteDC(hdc)
    记入成功/失败列表（连同失败原因，比如"密码错误"/"第 3 页渲染失败"/打印机报错）
return 汇总结果
```

**错误处理策略**：单个文件失败（无法解析/打印机中途报错）只影响该文件，记录原因后继续下一个；不中止整个批次。文件内部单页失败（比如某一页渲染出错）会中止该文件剩下的页码——`Result` 每个文件只存一个 `Err`，没有"部分页失败、部分页成功"这种中间状态可表达，与其悄悄漏印几页不如把整份文件标记为失败、让用户看到明确的失败原因；已经打印成功的前几页仍然会留在打印机队列里（`EndDoc` 依然会被调用，见下）。用户主动点「取消」则立即停止处理后续文件，已经开始的当前文件尽量走完 `EndDoc`（避免留下半截的打印任务卡在后台打印机队列里）。

**可测试性**：把上面伪代码里 `CreateDC`/`StartDoc`/`StartPage`/`StretchDIBits`/`EndPage`/`EndDoc`/`DeleteDC` 这几个 GDI 调用抽成一个小接口（`type printBackend interface {...}`），`RunPrintJob` 只依赖这个接口，不直接依赖 `lxn/win`。单元测试用假 backend（记录调用序列，可模拟"第 2 个文件 StartDoc 失败""收到进度回调后返回取消"等场景），覆盖"跳过失败继续""取消后停止""进度回调参数正确"这些编排逻辑，不需要真实打印机。真正的 GDI 调用只能手动测试（Windows 自带的"Microsoft Print to PDF"虚拟打印机可以在没有物理打印机的情况下验证管线整体走通）。

## 4. 进度反馈

- 点击「打印」后，`RunPrintJob` 丢进后台 goroutine 执行，主线程弹一个小的进度对话框：`walk.ProgressBar` + 「正在打印 2/5：xxx.pdf（第 3/12 页）」文字 + 「取消」按钮。
- 后台 goroutine 通过 `mainWindow.Synchronize` 把每次 `progressCB` 的回调结果搬回 UI 线程更新进度条/文字，与代码库里其它跨 goroutine 回 UI 线程的写法一致。
- 「取消」按钮设置一个 `bool`（或 `context.Context` 取消），下一次 `progressCB` 检查到即返回取消信号。
- 任务结束（正常完成或被取消）后，进度对话框关闭，弹 `walk.MsgBox` 汇总结果，例如：
  - 「打印完成：成功 4 个，失败 1 个（report.pdf：密码错误）」
  - 「已取消。成功 2 个，未打印：c.pdf、d.pdf」

## 5. 配置持久化（`internal/config`）

`Config` 结构体新增字段（与现有 `ContinuousMode`/`SidebarShown` 同样的直接加字段方式，`defaultConfig()` 给零值/空字符串）：

```go
LastPrinter      string  `json:"lastPrinter"`
LastGrayscale    bool    `json:"lastPrintGrayscale"`
LastDuplex       bool    `json:"lastPrintDuplex"`
LastPaperSize    string  `json:"lastPrintPaperSize"`
LastOrientation  string  `json:"lastPrintOrientation"`  // "portrait" / "landscape"
LastScaleMode    string  `json:"lastPrintScaleMode"`    // "fit" / "actual" / "percent"
LastScalePercent int     `json:"lastPrintScalePercent"`
```

打开打印对话框时用这些字段初始化设置面板；点击「打印」**发起**任务时立即写回并 `Save()`（不等结果——哪怕这次批量里有文件失败，设置本身是用户确认过的，下次应该记住）。范围（`rangeSpec`）不持久化，每次打开都是空列表、逐个文件重新选择。

## 6. 范围边界（这次明确不做）

- 不做实时预览面板。
- 不做"打印注释"（本阅读器无批注功能）。
- 不做长边/短边双面细分，只有开/关。
- 不做纸张自定义尺寸输入（只能从驱动枚举出的列表里选）。
- 不做"记住整个批次文件列表"这种持久化——每次打开打印对话框都从（可能为空的）当前标签页重新开始。
- 不做打印任务级别的后台队列/托盘常驻——对话框关闭前必须等当前批次跑完或被取消。

## 7. 测试策略

- `internal/print` 走 TDD：
  - 页码范围解析（`parseRange`）：空字符串→全部页、单页、逗号分隔、`"9-12"` 区间、超出该文件总页数的页码静默丢弃（不报错，因为不同文件页数不同、这本来就是常见情况）、非法字符（无法解析成数字/区间）报错、解析后一页都不剩（比如整段范围全部越界）时该文件计入失败列表，原因"页码范围无效"。
  - `RunPrintJob` 编排逻辑：用假 `printBackend`，覆盖"全部成功""某文件 CreateDC 失败后继续下一个""某页渲染失败后继续下一页""进度回调返回取消后停止处理后续文件""空文件列表"。
- `internal/config` 新增字段走现有 `TestSaveThenLoad_RoundTrip` 风格测试。
- `internal/ui` 部分不写自动化测试（与项目里其它 UI 任务一致），加入 README 手动测试清单：
  - [ ] 单个已打开文件：Ctrl+P 打开对话框，列表已预填当前文件，直接点「打印」能出纸（或用"Microsoft Print to PDF"验证）
  - [ ] 「添加PDF文件」多选新增文件，列表正确显示文件名和页数
  - [ ] 切换左侧不同文件时，「范围」输入框内容各自独立，其它设置项保持共享
  - [ ] 选择页范围（如 `1,3-5`）只打印对应页
  - [ ] 灰度/双面/纸张大小/方向/页面缩放各选项生效
  - [ ] 批量列表中一个文件密码错误/取消密码框，不影响其它文件继续打印，最后有汇总提示
  - [ ] 打印过程中点「取消」，后续文件不再打印，弹出正确的汇总
  - [ ] 关闭程序重新打开，打印设置（打印机/灰度/双面/纸张/方向/缩放）被记住
