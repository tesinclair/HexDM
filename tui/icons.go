package tui

var fprintIcon = `
     |
   \ | /
    \*/
---**O**---
    /*\
   / | \
     |
`

var fprintFailIcon = `
X   |   X
 X\ | /X
  X\*/X
--**X**--
  X/*\X
 X/ | \X
X   |   X
`

var fprintSuccessIcon = `
     |    //
   \ |   //
    \*/ //
\\-**O*//--
 \\ /*//
  \\ //
   \//
`

var fprintScanningFrames = []string{`
   .: | :.
  /   |   \
    \ | /    
 :-- -+- --:
    / | \   
  \   |   /
   ': | :'
`, `
   .: | :.
  /   |   \
    | / -
 :-- \+/ --:
    - \ |
  \   |   /
   ': | :'
`, `
   .: | :.
  /   |   \
    / - \
 :-- |+| --:
    \ - /
  \   |   /
   ': | :'
`, `
   .: | :.
  /   |   \
    - \ |
 :-- /+\ --:
    | / -
  \   |   /
   ': | :'
`}
