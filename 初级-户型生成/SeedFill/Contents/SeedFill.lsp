;; SeedFill - 文档初始化后自动 NETLOAD
(vl-load-com)

;; 保存已有的 S::STARTUP（如有）
(setq SeedFill:OldStartup (if (not (null S::STARTUP)) S::STARTUP nil))

;; 定义新的 S::STARTUP
(defun-q S::STARTUP ()
  ;; 先调用旧的 startup
  (if SeedFill:OldStartup (SeedFill:OldStartup))
  ;; 再 NETLOAD SeedFill
  (setq dllPath (strcat (getvar "ROAMABLEROOTPREFIX") "ApplicationPlugins\\SeedFill.bundle\\Contents\\SeedFill.dll"))
  (if (findfile dllPath)
    (progn
      (command "NETLOAD" dllPath)
      (princ "\nSeedFill 已加载。命令: Seed, Statistics")
    )
    (princ "\nSeedFill: 未找到 DLL")
  )
  (princ)
)

(princ)
