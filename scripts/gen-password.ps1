<#
.SYNOPSIS
Generate a strong password for Webiko Deployer.

.DESCRIPTION
Uses the OS cryptographic random generator, avoids ambiguous characters
(I, l, 0, 1, O), and guarantees upper case, lower case, digit and symbol.

.PARAMETER Length
Password length, 16 to 128. Default 32.

.PARAMETER Copy
Copy the password to the clipboard instead of printing it.

.EXAMPLE
powershell -ExecutionPolicy Bypass -File .\scripts\gen-password.ps1 -Copy

.EXAMPLE
powershell -ExecutionPolicy Bypass -File .\scripts\gen-password.ps1 -Length 48
#>
[CmdletBinding()]
param(
    [ValidateRange(16, 128)]
    [int]$Length = 32,

    [switch]$Copy
)

$ErrorActionPreference = 'Stop'

$upper   = 'ABCDEFGHJKLMNPQRSTUVWXYZ'
$lower   = 'abcdefghijkmnopqrstuvwxyz'
$digits  = '23456789'
$symbols = '!@#$%^&*-_=+?'
$chars   = $upper + $lower + $digits + $symbols

$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try {
    # Rejection sampling avoids modulo bias.
    $limit  = 256 - (256 % $chars.Length)
    $buffer = New-Object byte[] 64

    function New-Password {
        $builder = New-Object System.Text.StringBuilder
        while ($builder.Length -lt $Length) {
            $rng.GetBytes($buffer)
            foreach ($byte in $buffer) {
                if ($byte -lt $limit -and $builder.Length -lt $Length) {
                    [void]$builder.Append($chars[$byte % $chars.Length])
                }
            }
        }
        return $builder.ToString()
    }

    do {
        $password = New-Password
    } while (
        $password -cnotmatch '[A-Z]' -or
        $password -cnotmatch '[a-z]' -or
        $password -cnotmatch '[0-9]' -or
        $password -cnotmatch '[^A-Za-z0-9]'
    )
}
finally {
    $rng.Dispose()
}

if ($Copy) {
    Set-Clipboard -Value $password
    Write-Host 'Password copied to clipboard.'
}
else {
    Write-Output $password
}
