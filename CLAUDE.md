# 這個 repo 怎麼寫

**Parhelion PME**：8086 版 p-machine 直譯器（`SYSTEM.PME.86`）的知識庫，
以及照著它用 Go 重做一台機器。名字的由來寫在 `README.md`，
文件裡提到專案本身時用「Parhelion」，首次出現標中文「幻日」。
目標讀者是「要重做一台 p-machine，需要知道原版到底怎麼做」的人。

繼承 `~/.claude/rules/`（身分、文風、執行邊界）與 `~/.claude/rules/00-rules-index.md` 的
按需索引；本檔只寫這個 repo 特有的部分。

姊妹 repo：[`ucsd-pascal-notes`](https://github.com/wicanr2/ucsd-pascal-notes)
放 p-machine 的通則、指令編碼的第一性原理推導、IV.0 手冊摘譯與掃描檔。
**通則寫那邊，這一份實作的細節寫這邊。** 兩邊都要寫時，這邊引用那邊，不複製。

## 名字怎麼用

| 位置 | 用什麼 |
|---|---|
| repo | `Parhelion-PME86` |
| Go module | `github.com/wicanr2/Parhelion-PME86` |
| 執行檔 | `parhelion` |
| 文件標題與行文 | `Parhelion PME`；中文行文用「幻日」，首次出現標原文 |

Go 的 module path 帶大寫是合法的，module cache 會把大寫轉成 `!` 前綴的逃脫形式。
不要為了這件事把 repo 改成小寫——repo 名已經定案。

## 兩層結構

| 層 | 位置 | 性質 | 判準 |
|---|---|---|---|
| 知識層 | `docs/10-interpreter`、`docs/20-formats` | 從反組譯讀出來的事實與推導 | 每條斷言指得回檔案偏移或手冊頁碼 |
| 實作層 | `docs/30-remake`、（未來的）Go 程式碼 | 規格與程式碼 | 只有 `READY` 的 spec 授權正式實作 |

知識層是實作層的證據來源。新解出的東西先落進知識層，站得住才寫進 spec。

## 文件職責

| 檔 | 放什麼 | 不放什麼 |
|---|---|---|
| `README.md` | 用途、閱讀動線、資料來源、邊界宣告 | 日期、逐輪進度、失敗嘗試 |
| `README.en.md`／`README.ja.md` | 上一列的完整翻譯，不是摘要 | 只在某一語言成立的內容 |
| `*.en.md`／`*.ja.md` | 同名中文檔的翻譯，與它並排放 | — |
| `PLAN.md` | 分輪進度、待辦表、勘誤表 | 結論本身 |
| `CONTEXT.md` | 術語表與書寫慣例 | 知識內容 |
| `CLAUDE.md` | 本檔：工作契約 | 領域知識 |
| `docs/NN-主題/` | 一檔一主題，數字前綴決定閱讀順序 | — |
| `docs/30-remake/specs/` | 逐條配證據的規格，開頭標狀態與日期 | 沒有證據的想法 |
| `img/` | 所有 SVG | PNG（驗證用的走 scratchpad，不入版控） |
| `tools/` | 可重跑的證據管線 | 一次性分析腳本（走 scratchpad） |

新結論要寫進既有文件時，正文只寫現況，不敘述「當初怎麼錯的」；被推翻的舊斷言集中記在
`PLAN.md` 的勘誤表，正文最多留一個指標。教訓寫成規則，不寫成會過期的事件敘述
（見 `~/.claude/rulebook/63`）。

## spec 閘門

`DRAFT` → 證據審查 → `READY` → 實作 → 同狀態驗證 → `CONFORMED`。
細節在 `docs/30-remake/spec-workflow.md`。三條不可退讓：

- **只有 `READY` 授權正式實作。**
- 每條斷言標出處，出處只有三種：原版證據、推論（要寫推論步驟）、remake 自訂。
- **remake 自訂要再分「刻意不照做」與「原版沒有對應物」。** 混在一起就沒辦法
  回答「這裡跟原版不一樣，是 bug 還是設計」。

## 書寫規範

- 繁體中文。程式碼、助記符、識別字、檔名保留原文。
- 標點全形（`，。：；（）`）。半形只用在三處：程式碼區塊內、行內程式碼內、
  英文列舉與英文原名。
- 術語首次出現當場一句話翻譯。
- 結論先行。長篇開頭寫「結論先講：⋯」，接著才是推導。
- 不貼導引式 meta 標籤（「先看這段」「白話：」「本文適合⋯」）。章節用中性標題。
- 位址一律 `0x` 小寫十六進位，而且**寫明是檔案偏移還是執行期位址**。
- 繁體中文是正本；`*.en.md`／`*.ja.md` 是翻譯，**改中文的同輪要改翻譯**，
  每個檔頂端放一行語言切換。翻譯不是摘要：表格、程式碼、位址、待查證項照搬，
  只有敘述換語言——證據標記在三個語言裡是同一組字串，才回查得到同一處位元組。

## 證據紀律

- **每條結論標來源。** 反組譯結論給檔案偏移；手冊結論給**印刷頁碼**。
- **一手贏二手。** 原始位元組 > 反組譯推論 > 手冊 > 網路上流傳的表。
  手冊是 IV.0、檔案是 IV.2.1，兩者衝突時以檔案為準並記下差異。
- **推論等級。** 沒有一手來源支持的推論寫「待查證」或進 `PLAN.md` 開放項目，
  不寫進正文的表。
- **反組譯只涵蓋主路徑。** 逐支反組譯的結束判準是第一個無條件轉移。
  「這支只有 8 條指令」不等於「這支只做這 8 件事」。
- **接手別人的產出，先驗完整性再驗形式。** 一份文件宣稱涵蓋什麼範圍，就核對它
  是不是真的寫到那裡；形式檢查全過不代表內容沒有斷在一半。
- **統計數字要寫清楚是怎麼數的。** 「出現 19 次」是數位元組序列，
  「169 支常式」是數 dispatch 表的相異值——兩者受不同的偏誤影響。

## 配圖

- 概念圖、格式圖、記憶體佈局一律手繪 SVG 放 `img/`，不用 ASCII 圖。
- 白底、克制配色（可帶橘色點綴）、圓角框，
  `font-family="'Noto Sans CJK TC','PingFang TC','Microsoft JhengHei',sans-serif"`。
- 畫完一定轉 PNG 自己看過再收，檢查 CJK 缺字、標籤重疊、溢出：

  ```
  docker run --rm --network none -u "$(id -u):$(id -g)" \
    -v "$PWD:/w" -w /w zenika/alpine-chrome \
    --headless --no-sandbox --disable-gpu --screenshot=/w/out.png \
    --window-size=W,H --force-device-scale-factor=2 file:///w/img/x.svg
  ```

- 文件引用相對路徑（`docs/NN-主題/` 深度 2 → `../../img/`），
  用 `<p align="center"><img src=... width=... alt=...></p>` 置中。

## 每輪流程

1. 一次推進一個主題。
2. 寫文件 → 配圖 → 轉 PNG 自檢。
3. 更新 `README.md` 動線、`CONTEXT.md` 術語、`PLAN.md` 進度與待辦。
4. 檢查有沒有舊斷言被這輪的結論推翻；有就同輪改掉。
5. `git add -A` → 繁中 commit → push。

## 執行環境

- 反組譯、批次文字處理、轉檔、抓圖一律在 Docker 容器內跑；主機只做 git、
  檔案編輯與容器控制。一次性工作用
  `docker run --rm --network none -u "$(id -u):$(id -g)"`
  並設 `--memory` / `--cpus` / `--pids-limit` 與 `--log-opt max-size=10m`。
- **不碰共用的 docker 資源**：禁止任何 `prune`、`rmi`、刪除不是自己這次建立的容器。
- 反組譯用 `ida-pro-9.4-idapython:locked-v1`；headless 的輸出不進 stdout，
  **一律寫檔並驗檔案存在、非空、schema 對、輸入 sha256 對**；exit code 不能當證據。
  作法見 `~/.claude/knowledge-base/retro/ida-pro-9.4.md`。
- Go 一律在容器內建置與測試，不裝主機工具鏈。
- 中間產物寫 scratchpad，不寫進 repo。

## git

- 作者信箱一律 `wicanr2@gmail.com`。進 repo 動工時先跑
  `git config user.email` 與 `git log --format=%ae | sort -u` 各一次
  （前者管下一個 commit，後者才看得到歷史）。
- commit message 用繁體中文，結尾帶 `Co-Authored-By:`。
  **不放 `Claude-Session:` 連結。**
- push 前確認 `README.md` 的邊界宣告與 repo 實際內容一致（這是 public repo）。

## 素材位置

| 東西 | 位置 |
|---|---|
| `SYSTEM.PME.86`（16384 B，sha256 `fe427aa6…`） | `~/cht/p-code/psys21/` 抽出，不入版控 |
| p-System IV.2.1 磁碟映像（1984） | `~/cht/p-code/psys21/`，不入版控 |
| IV.0 Internal Architecture Guide 掃描檔與摘譯 | `ucsd-pascal-notes` 的 `refs/` 與 `docs/50-iv-internals/` |
| 68000 版直譯器的對照結論 | `ucsd-pascal-notes` 的 `docs/30-opcode-tables/` |
