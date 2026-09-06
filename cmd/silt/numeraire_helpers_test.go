package main

import "github.com/nerolabs/silt/core/credit"

// econMintUnit is one object-path mint unit (G-R212-7): SkimDen·Dλ bytes served on a
// lane mints exactly SkimDen−SkimNum credits net and SkimNum skimmed, so fixtures can
// be written in credits.
const econMintUnit = int64(credit.SkimDen * credit.ServeMintBytesPerCredit)
