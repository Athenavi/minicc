;; 地下室车位排布插件 - ParkingSolver
;; 通过 Bundle 自动加载
(vl-load-com)
(princ "\n加载 ParkingSolver...")

;; 命令: GTCW
(defun C:GTCW (/ dllPath result cmd msg)
  (princ "\n=== 地下室车位排布 GTCW ===")
  
  ;; 搜索可能的路径
  (setq searchPaths 
    (list
      (strcat (getvar "ROAMABLEROOTPREFIX") "ApplicationPlugins\\ParkingSolver.Bundle\\Contents\\")
      (strcat (getvar "LOCALROOTPREFIX") "ApplicationPlugins\\ParkingSolver.Bundle\\Contents\\")
      (getvar "DWGPREFIX")
    )
  )
  
  (setq dllPath nil)
  (foreach p searchPaths
    (setq testPath (strcat p "ParkingSolver.dll"))
    (if (findfile testPath)
      (setq dllPath testPath)
    )
  )
  
  (if (null dllPath)
    (progn
      (princ "\n错误: 未找到 ParkingSolver.dll")
      (princ (strcat "\n查找路径: " (car searchPaths)))
      (princ)
      (exit)
    )
  )
  
  (princ (strcat "\n找到 DLL: " dllPath))
  
  ;; NETLOAD 加载
  (command "NETLOAD" dllPath)
  
  ;; 通过 COM 调用
  (setq result 
    (vl-catch-all-apply
      '(lambda ()
        (setq cmd (vlax-create-object "ParkingSolver.Command"))
        (setq msg (vlax-invoke-method cmd "Execute"))
        (vlax-release-object cmd)
        msg
      )
    )
  )
  
  (if (vl-catch-all-error-p result)
    (princ (strcat "\n出错: " (vl-catch-all-error-message result)))
    (princ (strcat "\n" result))
  )
  
  (princ)
)

(princ "\n地下室车位排布插件已加载。输入 GTCW 执行。")
(princ)
