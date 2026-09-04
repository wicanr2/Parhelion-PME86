# 定址：從 p-code 的 word 世界換到 8086 的位元組世界

p-code 的位址單位是 word，變數編號從 1 開始。8086 的位址單位是位元組。
這一篇看的是兩者之間那一層換算，以及活動記錄的實際版面。

> 延伸閱讀：[直譯器的狀態](machine-state.md)｜[段的切換](segment-switching.md)

## 變長運算元：同一段碼抄了七次

手冊 p.16 的 big operand 規則是：**第一個位元組最高位為 0 就是它本身（0–127），
為 1 則取低 7 位當高位元組，再讀一個位元組當低位元組（0–32767）。**
8086 版的實作每次都原地展開：

```
ac        lodsb
98        cbw
84c0      test al, al
7905      jns  +5              ; 最高位為 0 → 直接用
247f      and  al, 7Fh
86e0      xchg ah, al          ; 低 7 位變成高位元組
ac        lodsb                ; 再讀一個當低位元組
d1e0      shl  ax, 1           ; ← word 偏移 ×2 = 位元組偏移
```

這 9 個位元組的序列（`84 c0 79 05 24 7f 86 e0 ac`）在檔案裡出現 **19 次**，字面完全相同。
`LCO`、`LDC`、`LAO`、`XJP`、`RPU`、`MOV` 與兩支定址助手都在其中。
沒有做成子程式的理由，從碼本身看得出來：整段 15 個位元組，
換成 `call` 加 `retn` 要 4 個位元組加兩次轉移的時間——**在最熱的路徑上，
呼叫的代價比重複的位元組貴。**

最後那句 `shl ax, 1` 是整個直譯器出現 160 次的動作。p-code 說「第 5 個 word」，
8086 要的是「第 10 個位元組」。

## 活動記錄：為什麼基底都要加 8

`MOV`／`LOD`／`STR` 這一族存取局部變數時，位址是 `bx + 8 + 2×編號`。
全域則是 `dx + 2×編號`，而 `dx` 在切段時被設成 `Env_Data + 8`。
兩邊都加了 8——加的是 Mark Stack Control Word 的長度。

手冊 Figure 5 給的 MSCW 欄位順序是 `MSSTAT`、`MSDYN`、`MSIPC`、`MSENV`、`MSPROC`，
五個 word。`RPU` 把它整批彈出來，順序一目了然：

```
1140: a12e00    mov ax, word_2E      ; MP
1143: 48 48     dec ax; dec ax       ; sp ← MP−2
1145: 8be0      mov sp, ax
1147: 5f        pop di               ; [MP−2]
1148: 5b        pop bx               ; [MP+0]  MSSTAT
1149: 5b        pop bx               ; [MP+2]  MSDYN  → 新的 MP
114a: 891e2e00  mov word_2E, bx
114e: 83c308    add bx, 8            ; 新的局部資料基底
1151: 891e2600  mov word_26, bx
1155: 5e        pop si               ; [MP+4]  MSIPC
1156: 58        pop ax               ; [MP+6]  MSENV
1157: 5d        pop bp               ; [MP+8]  MSPROC
1158: 0326d400  add sp, word_D4      ; 削掉參數
115c: 892e3200  mov word_32, bp
1160: 85ed      test bp, bp
1162: 7811      js  ...              ; 程序號為負 → 走 EXITIC
```

所以基底加 8 之後，**編號 1 的變數落在 `MP+10`，正好是 MSCW 之後第一個 word**。
p-code 的變數編號從 1 起算不是慣例問題，是版面算出來的結果。

`test bp, bp; js` 那兩句對上手冊對 `RPU` 的描述：「若 MSCW 裡的程序號 < 0，
返回該程序的 EXITIC 而不是 MSCW 的 IPC」。

<p align="center"><img src="../../img/activation-record.svg" width="960" alt="活動記錄版面：MSCW 五個 word 佔 MP+0 到 MP+8，區域變數從 MP+10 開始，所以基底暫存器 bx 是 MP+8"></p>

## 中介層變數：走靜態鏈

`LOD`／`STR`／`LDA` 的第一個運算元是層級差 `DB`。助手 `0x093a`：

```
093a: 32e4      xor  ah, ah
093c: ac        lodsb                ; DB
093d: 91        xchg ax, cx
093e: 8beb      mov  bp, bx          ; ← SLOD1 / SLOD2 從這裡進來
0940: 83ed08    sub  bp, 8           ; 回到 MP
0943: e305      jcxz +5
0945: 8b6e00    mov  bp, [bp+0]      ; MSSTAT：往外一層
0948: e2fb      loop
094a: <big operand 解碼>
0955: d1e0      shl  ax, 1
0957: 03e8      add  bp, ax
0959: 83c508    add  bp, 8
095c: c3        retn
```

`bx` 是加了 8 的基底，要走鏈就得先減回去，走完再加回來。
`SLOD1`、`SLOD2` 是「層級差固定為 1 或 2」的短形式，它們自己把 `cx` 設好，
**從 `0x093e` 進來跳過取 `DB` 那三個位元組**——同一段碼兩個入口。

跨段的全域（`LAE`／`LDE`／`STE`）走另一條：

```
15d2: 32e4      xor ah, ah
15d4: ac        lodsb                ; segment number
15d5: 95        xchg ax, bp
15d6: d1e5      shl bp, 1
15d8: 368b3e3a00 mov di, ss:3Ah      ; E_Vec
15dd: 8b2b      mov bp, [bp+di]      ; E_Vec[seg] → E_Rec
15df: 8b6e00    mov bp, [bp+0]       ; E_Rec.Env_Data
15e2: <big operand 解碼>
15ef: 050800    add ax, 8
15f2: 03e8      add bp, ax
```

同樣的「加 8」。差別只在基底是從 `E_Vec` 查出來的，不是常駐的 `dx`。

## packed 欄位：查表比算便宜

手冊 p.47 把 packed field 的位址定義成堆疊上的三個 word：位址、位元數、最右 bit 編號。
8086 版的 `LDP`：

```
0a77: 59        pop cx               ; 最右 bit 編號
0a78: 58        pop ax               ; 位元數
0a79: 5d        pop bp               ; word 位址
0a7a: 8b6e00    mov bp, [bp+0]
0a7d: d3ed      shr bp, cl           ; 欄位移到最低位
0a7f: 95        xchg ax, bp
0a80: d1e5      shl bp, 1            ; 位元數 ×2 當索引
0a82: 2e2386b61f and ax, cs:[bp+1FB6h]
```

`0x1fb6` 起是一張現成的遮罩表：

```
0000 0001 0003 0007 000f 001f 003f 007f
00ff 01ff 03ff 07ff 0fff 1fff 3fff 7fff ffff
```

第 n 項就是 `(1 << n) − 1`。`STP` 用同一張表取遮罩，再用 `ror`／`rol` 把欄位轉到定位：

```
0a91: 5f        pop di               ; 要存的資料
0a92: 59        pop cx               ; 最右 bit 編號
0a93: 5d        pop bp               ; 位元數
0a94: d1e5      shl bp, 1
0a96: 2e8b86b61f mov ax, cs:[bp+1FB6h]
0a9b: 23f8      and di, ax           ; 資料截到欄位寬
0a9d: f7d0      not ax               ; 反遮罩
0a9f: 5d        pop bp               ; word 位址
0aa0: 8b5600    mov dx, [bp+0]
0aa3: d3ca      ror dx, cl           ; 欄位轉到最低位
0aa5: 23d0      and dx, ax           ; 清掉欄位
0aa7: 0bd7      or  dx, di           ; 填新值
0aa9: d3c2      rol dx, cl           ; 轉回去
0aab: 895600    mov [bp+0], dx
0aae: 368b162800 mov dx, ss:28h      ; 補回全域基底
```

`ror`／`rol` 一對比「左移再右移」少一次移位，代價是要多一個反遮罩——
而反遮罩只花一條 `not`。

同版本的 68000 直譯器沒有這張表，改用「左移再右移，用移位器當遮罩產生器」。
兩邊都在避開「執行期算遮罩」，因為欄位寬度是執行期的值；手段由指令集決定。

`IXP` 把「基底加索引」變成那個三元組，兩個 CPU 都用一次除法同時得到商與餘數：

```
0a50: 32e4 ac   xor ah,ah; lodsb     ; UB_1：每個 word 幾個元素
0a53: 95        xchg ax, bp
0a54: 32e4 ac   xor ah,ah; lodsb     ; UB_2：欄位位元數
0a57: 91        xchg ax, cx
0a58: 58        pop ax               ; 索引
0a59: 5f        pop di               ; 基底
0a5a: 33d2      xor dx, dx
0a5c: f7f5      div bp               ; ax = 第幾個 word，dx = 第幾格
0a5e: d1e0      shl ax, 1
0a60: 03f8      add di, ax
0a62: 57        push di              ; 三元組：位址
0a63: 51        push cx              ;         位元數
```

## 邊界

- 「19 次」是在整份檔案裡數位元組序列得到的，不受反組譯截斷影響；但序列比對只認完全相同的寫法，變形的解碼不會被算進去。
- `LDP`／`STP` 的堆疊次序與手冊 `Pack-ptr` 的定義逐項相同，這一點兩個 CPU 版都驗過。
- `IXP` 用除法算「第幾個 word」，隱含「欄位不跨 word 邊界」。編譯器怎麼保證這件事，
  本篇沒有追。
