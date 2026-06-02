# Third-Party Notices

Gebweb includes and may redistribute code from the third-party Go modules
listed below. This notice was prepared from `go.mod`, `go.sum`, and the
license files present in the local Go module cache. No
`docs/dependency-licenses` directory or file was present in this checkout.

## Dependency Inventory

The following modules are declared in `go.mod` and covered by this notice:

| Module | Version | License / notice basis |
| --- | --- | --- |
| `filippo.io/edwards25519` | v1.2.0 | BSD-3-Clause style |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT |
| `github.com/fsnotify/fsnotify` | v1.10.1 | BSD-3-Clause style |
| `github.com/go-sql-driver/mysql` | v1.10.0 | MPL-2.0 |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause style |
| `github.com/jackc/pgpassfile` | v1.0.0 | MIT |
| `github.com/jackc/pgservicefile` | v0.0.0-20240606120523-5a60cdf6a761 | MIT |
| `github.com/jackc/pgx/v5` | v5.9.2 | MIT |
| `github.com/jackc/puddle/v2` | v2.2.2 | MIT |
| `github.com/kr/text` | v0.2.0 | MIT |
| `github.com/mattn/go-isatty` | v0.0.20 | MIT |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause style |
| `github.com/rogpeppe/go-internal` | v1.15.0 | BSD-3-Clause style |
| `golang.org/x/sync` | v0.20.0 | BSD-3-Clause style |
| `golang.org/x/sys` | v0.42.0 | BSD-3-Clause style |
| `golang.org/x/text` | v0.29.0 | BSD-3-Clause style |
| `gopkg.in/yaml.v3` | v3.0.1 | MIT and Apache-2.0 |
| `modernc.org/libc` | v1.72.3 | BSD-3-Clause style, plus bundled third-party notices |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause style |
| `modernc.org/memory` | v1.11.0 | BSD-3-Clause style, plus bundled Go and mmap-go notices |
| `modernc.org/sqlite` | v1.50.1 | BSD-3-Clause style |

The `go.sum` file also records modules that are not declared dependencies of
this module, such as historical or test-only checksum entries. Those entries
were reviewed but are not listed here as redistributed dependencies.

## BSD-3-Clause Style Notices

The following modules use BSD-3-Clause style licenses. Their copyright notices
are reproduced with the applicable license terms:

* `filippo.io/edwards25519`: Copyright (c) 2009 The Go Authors. All rights reserved.
* `github.com/fsnotify/fsnotify`: Copyright (c) 2012 The Go Authors. All rights reserved. Copyright (c) fsnotify Authors. All rights reserved.
* `github.com/google/uuid`: Copyright (c) 2009,2014 Google Inc. All rights reserved.
* `github.com/remyoudompheng/bigfft`: Copyright (c) 2012 The Go Authors. All rights reserved.
* `github.com/rogpeppe/go-internal`: Copyright (c) 2018 The Go Authors. All rights reserved.
* `golang.org/x/sync`: Copyright 2009 The Go Authors.
* `golang.org/x/sys`: Copyright 2009 The Go Authors.
* `golang.org/x/text`: Copyright 2009 The Go Authors.
* `modernc.org/libc`: Copyright (c) 2017 The Libc Authors. All rights reserved.
* `modernc.org/mathutil`: Copyright (c) 2014 The mathutil Authors. All rights reserved.
* `modernc.org/mathutil/mersenne`: Copyright (c) 2014 The mersenne Authors. All rights reserved.
* `modernc.org/memory`: Copyright (c) 2017 The Memory Authors. All rights reserved.
* `modernc.org/memory` Go-derived portions: Copyright (c) 2009 The Go Authors. All rights reserved.
* `modernc.org/memory` mmap-go portions: Copyright (c) 2011, Evan Shaw <edsrzf@gmail.com>. All rights reserved.
* `modernc.org/sqlite`: Copyright (c) 2017 The Sqlite Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software
   without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

## MIT Notices

The following modules use MIT or MIT-style licenses. Their copyright notices
are reproduced with the applicable license terms:

* `github.com/dustin/go-humanize`: Copyright (c) 2005-2008 Dustin Sallings <dustin@spy.net>
* `github.com/jackc/pgpassfile`: Copyright (c) 2019 Jack Christensen
* `github.com/jackc/pgservicefile`: Copyright (c) 2020 Jack Christensen
* `github.com/jackc/pgx/v5`: Copyright (c) 2013-2021 Jack Christensen
* `github.com/jackc/puddle/v2`: Copyright (c) 2018 Jack Christensen
* `github.com/kr/text`: Copyright 2012 Keith Rarick
* `github.com/mattn/go-isatty`: Copyright (c) Yasuhiro MATSUMOTO <mattn.jp@gmail.com>
* `github.com/ncruces/go-strftime`: Copyright (c) 2022 Nuno Cruces

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## gopkg.in/yaml.v3

This project is covered by two different licenses: MIT and Apache.

The following files were ported to Go from C files of libyaml, and thus are
still covered by their original MIT license, with the additional copyright
starting in 2011 when the project was ported over:

    apic.go emitterc.go parserc.go readerc.go scannerc.go
    writerc.go yamlh.go yamlprivateh.go

Copyright (c) 2006-2010 Kirill Simonov
Copyright (c) 2006-2011 Kirill Simonov

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

All remaining project files are covered by the Apache License:

Copyright (c) 2011-2019 Canonical Ltd

Licensed under the Apache License, Version 2.0 (the "License"); you may not
use this file except in compliance with the License. You may obtain a copy of
the License at:

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
License for the specific language governing permissions and limitations under
the License.

## github.com/go-sql-driver/mysql

`github.com/go-sql-driver/mysql` is licensed under the Mozilla Public License
Version 2.0. Its upstream license notice is:

This Source Code Form is subject to the terms of the Mozilla Public License,
v. 2.0. If a copy of the MPL was not distributed with this file, You can
obtain one at http://mozilla.org/MPL/2.0/.

## modernc.org/libc Third-Party Software Notices

`modernc.org/libc` contains code and assets acquired from third-party sources.
The upstream notice lists the following components:

### Go

URL: https://github.com/golang/go

Copyright (c) 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of Google Inc. nor the names of its contributors may be
   used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

### musl libc

URL: https://musl.libc.org/

musl as a whole is licensed under the following standard MIT license:

Copyright (c) 2005-2020 Rich Felker, et al.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Authors/contributors include:

A. Wilcox, Ada Worcester, Alex Dowad, Alex Suykov, Alexander Monakov, Andre
McCurdy, Andrew Kelley, Anthony G. Basile, Aric Belsito, Arvid Picciani,
Bartosz Brachaczek, Benjamin Peterson, Bobby Bingham, Boris Brezillon, Brent
Cook, Chris Spiegel, Clement Vasseur, Daniel Micay, Daniel Sabogal,
Daurnimator, David Carlier, David Edelsohn, Denys Vlasenko, Dmitry Ivanov,
Dmitry V. Levin, Drew DeVault, Emil Renner Berthing, Fangrui Song, Felix
Fietkau, Felix Janda, Gianluca Anzolin, Hauke Mehrtens, He X, Hiltjo
Posthuma, Isaac Dunham, Jaydeep Patil, Jens Gustedt, Jeremy Huntwork,
Jo-Philipp Wich, Joakim Sindholt, John Spencer, Julien Ramseier, Justin
Cormack, Kaarle Ritvanen, Khem Raj, Kylie McClain, Leah Neukirchen, Luca
Barbato, Luka Perkov, M Farkas-Dyck (Strake), Mahesh Bodapati, Markus
Wichmann, Masanori Ogino, Michael Clark, Michael Forney, Mikhail Kremnyov,
Natanael Copa, Nicholas J. Kain, orc, Pascal Cuoq, Patrick Oppenlander, Petr
Hosek, Petr Skocik, Pierre Carrier, Reini Urban, Rich Felker, Richard
Pennington, Ryan Fairfax, Samuel Holland, Segev Finer, Shiz, sin, Solar
Designer, Stefan Kristiansson, Stefan O'Rear, Szabolcs Nagy, Timo Teras,
Trutz Behn, Valentin Ochs, Will Dietz, William Haddon, William Pitcock.

Portions of musl are derived from third-party works licensed under terms
compatible with the MIT license above, including:

* TRE regular expression implementation: Copyright (c) 2001-2008 Ville
  Laurikari, licensed under a 2-clause BSD license.
* Math library code: Copyright (c) 1993,2004 Sun Microsystems; Copyright (c)
  2003-2011 David Schultz; Copyright (c) 2003-2009 Steven G. Kargl; Copyright
  (c) 2003-2009 Bruce D. Evans; Copyright (c) 2008 Stephen L. Moshier;
  Copyright (c) 2017-2018 Arm Limited.
* ARM memcpy code: Copyright (c) 2008 The Android Open Source Project,
  licensed under a two-clause BSD license.
* AArch64 memcpy and memset code: Copyright (c) 1999-2019 Arm Limited.
* DES crypt implementation: Copyright (c) 1994 David Burren, licensed under a
  BSD license.
* Smoothsort implementation: Copyright (c) 2011 Valentin Ochs, licensed under
  an MIT-style license.
* x86_64 port: Nicholas J. Kain, licensed under standard MIT terms.
* mips and microblaze ports: Richard Pennington, adapted by Rich Felker,
  licensed under standard MIT terms.
* mips64 port: Imagination Technologies, licensed under standard MIT terms.
* powerpc port: Richard Pennington and John Spencer, licensed under standard
  MIT terms.

For the two-clause BSD portions identified above, the applicable license terms
are:

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

Public header files and crt files intended to be linked into applications may
omit the copyright notice and permission notice otherwise required by the
license. These files include substantial contributions from Bobby Bingham,
John Spencer, Nicholas J. Kain, Rich Felker, Richard Pennington, Stefan
Kristiansson, and Szabolcs Nagy, all of whom explicitly granted such
permission.

### go-netdb

URL: https://github.com/dominikh/go-netdb

Copyright (c) 2012 Dominik Honnef

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

### NixOS/nixpkgs

URL: https://github.com/NixOS/nixpkgs

Copyright (c) 2003-2025 Eelco Dolstra and the Nixpkgs/NixOS contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
