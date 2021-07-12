// Code generated by running "go generate" in golang.org/x/text. DO NOT EDIT.

package language

// CLDRVersion is the CLDR version from which the tables in this package are derived.
const CLDRVersion = "32"

const (
	_de  = 269
	_en  = 313
	_fr  = 350
	_it  = 505
	_mo  = 784
	_no  = 879
	_nb  = 839
	_pt  = 960
	_sh  = 1031
	_mul = 806
	_und = 0
)

const (
	_001 = 1
	_419 = 31
	_BR  = 65
	_CA  = 73
	_ES  = 110
	_GB  = 123
	_MD  = 188
	_PT  = 238
	_UK  = 306
	_US  = 309
	_ZZ  = 357
	_XA  = 323
	_XC  = 325
	_XK  = 333
)

const (
	_Latn = 90
	_Hani = 57
	_Hans = 59
	_Hant = 60
	_Qaaa = 143
	_Qaai = 151
	_Qabx = 192
	_Zinh = 245
	_Zyyy = 250
	_Zzzz = 251
)

var regionToGroups = []uint8{ // 358 elements
	// Entry 0 - 3F
	0x00, 0x00, 0x00, 0x04, 0x04, 0x00, 0x00, 0x04,
	0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x04, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x00,
	0x00, 0x04, 0x00, 0x00, 0x04, 0x01, 0x00, 0x00,
	0x04, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x00, 0x04,
	// Entry 40 - 7F
	0x04, 0x04, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x04, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x04, 0x00, 0x00, 0x04, 0x00, 0x04, 0x00,
	0x00, 0x04, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x00, 0x08,
	0x00, 0x04, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x04, 0x00,
	// Entry 80 - BF
	0x00, 0x00, 0x04, 0x00, 0x00, 0x04, 0x00, 0x00,
	0x00, 0x04, 0x01, 0x00, 0x04, 0x02, 0x00, 0x04,
	0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x04, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00,
	0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x08, 0x08, 0x00, 0x00, 0x00, 0x04, 0x00,
	// Entry C0 - FF
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01,
	0x04, 0x08, 0x04, 0x00, 0x00, 0x00, 0x00, 0x04,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x04, 0x00, 0x04, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x04, 0x00, 0x05, 0x00, 0x00, 0x00,
	0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Entry 100 - 13F
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00,
	0x00, 0x00, 0x04, 0x04, 0x00, 0x00, 0x00, 0x04,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x08, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x05, 0x04, 0x00,
	0x00, 0x04, 0x00, 0x04, 0x04, 0x05, 0x00, 0x00,
	// Entry 140 - 17F
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
} // Size: 382 bytes

var paradigmLocales = [][3]uint16{ // 3 elements
	0: {0x139, 0x0, 0x7b},
	1: {0x13e, 0x0, 0x1f},
	2: {0x3c0, 0x41, 0xee},
} // Size: 42 bytes

type mutualIntelligibility struct {
	want     uint16
	have     uint16
	distance uint8
	oneway   bool
}

type scriptIntelligibility struct {
	wantLang   uint16
	haveLang   uint16
	wantScript uint8
	haveScript uint8
	distance   uint8
}

type regionIntelligibility struct {
	lang     uint16
	script   uint8
	group    uint8
	distance uint8
}

// matchLang holds pairs of langIDs of base languages that are typically
// mutually intelligible. Each pair is associated with a confidence and
// whether the intelligibility goes one or both ways.
var matchLang = []mutualIntelligibility{ // 113 elements
	0:   {want: 0x1d1, have: 0xb7, distance: 0x4, oneway: false},
	1:   {want: 0x407, have: 0xb7, distance: 0x4, oneway: false},
	2:   {want: 0x407, have: 0x1d1, distance: 0x4, oneway: false},
	3:   {want: 0x407, have: 0x432, distance: 0x4, oneway: false},
	4:   {want: 0x43a, have: 0x1, distance: 0x4, oneway: false},
	5:   {want: 0x1a3, have: 0x10d, distance: 0x4, oneway: true},
	6:   {want: 0x295, have: 0x10d, distance: 0x4, oneway: true},
	7:   {want: 0x101, have: 0x36f, distance: 0x8, oneway: false},
	8:   {want: 0x101, have: 0x347, distance: 0x8, oneway: false},
	9:   {want: 0x5, have: 0x3e2, distance: 0xa, oneway: true},
	10:  {want: 0xd, have: 0x139, distance: 0xa, oneway: true},
	11:  {want: 0x16, have: 0x367, distance: 0xa, oneway: true},
	12:  {want: 0x21, have: 0x139, distance: 0xa, oneway: true},
	13:  {want: 0x56, have: 0x13e, distance: 0xa, oneway: true},
	14:  {want: 0x58, have: 0x3e2, distance: 0xa, oneway: true},
	15:  {want: 0x71, have: 0x3e2, distance: 0xa, oneway: true},
	16:  {want: 0x75, have: 0x139, distance: 0xa, oneway: true},
	17:  {want: 0x82, have: 0x1be, distance: 0xa, oneway: true},
	18:  {want: 0xa5, have: 0x139, distance: 0xa, oneway: true},
	19:  {want: 0xb2, have: 0x15e, distance: 0xa, oneway: true},
	20:  {want: 0xdd, have: 0x153, distance: 0xa, oneway: true},
	21:  {want: 0xe5, have: 0x139, distance: 0xa, oneway: true},
	22:  {want: 0xe9, have: 0x3a, distance: 0xa, oneway: true},
	23:  {want: 0xf0, have: 0x15e, distance: 0xa, oneway: true},
	24:  {want: 0xf9, have: 0x15e, distance: 0xa, oneway: true},
	25:  {want: 0x100, have: 0x139, distance: 0xa, oneway: true},
	26:  {want: 0x130, have: 0x139, distance: 0xa, oneway: true},
	27:  {want: 0x13c, have: 0x139, distance: 0xa, oneway: true},
	28:  {want: 0x140, have: 0x151, distance: 0xa, oneway: true},
	29:  {want: 0x145, have: 0x13e, distance: 0xa, oneway: true},
	30:  {want: 0x158, have: 0x101, distance: 0xa, oneway: true},
	31:  {want: 0x16d, have: 0x367, distance: 0xa, oneway: true},
	32:  {want: 0x16e, have: 0x139, distance: 0xa, oneway: true},
	33:  {want: 0x16f, have: 0x139, distance: 0xa, oneway: true},
	34:  {want: 0x17e, have: 0x139, distance: 0xa, oneway: true},
	35:  {want: 0x190, have: 0x13e, distance: 0xa, oneway: true},
	36:  {want: 0x194, have: 0x13e, distance: 0xa, oneway: true},
	37:  {want: 0x1a4, have: 0x1be, distance: 0xa, oneway: true},
	38:  {want: 0x1b4, have: 0x139, distance: 0xa, oneway: true},
	39:  {want: 0x1b8, have: 0x139, distance: 0xa, oneway: true},
	40:  {want: 0x1d4, have: 0x15e, distance: 0xa, oneway: true},
	41:  {want: 0x1d7, have: 0x3e2, distance: 0xa, oneway: true},
	42:  {want: 0x1d9, have: 0x139, distance: 0xa, oneway: true},
	43:  {want: 0x1e7, have: 0x139, distance: 0xa, oneway: true},
	44:  {want: 0x1f8, have: 0x139, distance: 0xa, oneway: true},
	45:  {want: 0x20e, have: 0x1e1, distance: 0xa, oneway: true},
	46:  {want: 0x210, have: 0x139, distance: 0xa, oneway: true},
	47:  {want: 0x22d, have: 0x15e, distance: 0xa, oneway: true},
	48:  {want: 0x242, have: 0x3e2, distance: 0xa, oneway: true},
	49:  {want: 0x24a, have: 0x139, distance: 0xa, oneway: true},
	50:  {want: 0x251, have: 0x139, distance: 0xa, oneway: true},
	51:  {want: 0x265, have: 0x139, distance: 0xa, oneway: true},
	52:  {want: 0x274, have: 0x48a, distance: 0xa, oneway: true},
	53:  {want: 0x28a, have: 0x3e2, distance: 0xa, oneway: true},
	54:  {want: 0x28e, have: 0x1f9, distance: 0xa, oneway: true},
	55:  {want: 0x2a3, have: 0x139, distance: 0xa, oneway: true},
	56:  {want: 0x2b5, have: 0x15e, distance: 0xa, oneway: true},
	57:  {want: 0x2b8, have: 0x139, distance: 0xa, oneway: true},
	58:  {want: 0x2be, have: 0x139, distance: 0xa, oneway: true},
	59:  {want: 0x2c3, have: 0x15e, distance: 0xa, oneway: true},
	60:  {want: 0x2ed, have: 0x139, distance: 0xa, oneway: true},
	61:  {want: 0x2f1, have: 0x15e, distance: 0xa, oneway: true},
	62:  {want: 0x2fa, have: 0x139, distance: 0xa, oneway: true},
	63:  {want: 0x2ff, have: 0x7e, distance: 0xa, oneway: true},
	64:  {want: 0x304, have: 0x139, distance: 0xa, oneway: true},
	65:  {want: 0x30b, have: 0x3e2, distance: 0xa, oneway: true},
	66:  {want: 0x31b, have: 0x1be, distance: 0xa, oneway: true},
	67:  {want: 0x31f, have: 0x1e1, distance: 0xa, oneway: true},
	68:  {want: 0x320, have: 0x139, distance: 0xa, oneway: true},
	69:  {want: 0x331, have: 0x139, distance: 0xa, oneway: true},
	70:  {want: 0x351, have: 0x139, distance: 0xa, oneway: true},
	71:  {want: 0x36a, have: 0x347, distance: 0xa, oneway: false},
	72:  {want: 0x36a, have: 0x36f, distance: 0xa, oneway: true},
	73:  {want: 0x37a, have: 0x139, distance: 0xa, oneway: true},
	74:  {want: 0x387, have: 0x139, distance: 0xa, oneway: true},
	75:  {want: 0x389, have: 0x139, distance: 0xa, oneway: true},
	76:  {want: 0x38b, have: 0x15e, distance: 0xa, oneway: true},
	77:  {want: 0x390, have: 0x139, distance: 0xa, oneway: true},
	78:  {want: 0x395, have: 0x139, distance: 0xa, oneway: true},
	79:  {want: 0x39d, have: 0x139, distance: 0xa, oneway: true},
	80:  {want: 0x3a5, have: 0x139, distance: 0xa, oneway: true},
	81:  {want: 0x3be, have: 0x139, distance: 0xa, oneway: true},
	82:  {want: 0x3c4, have: 0x13e, distance: 0xa, oneway: true},
	83:  {want: 0x3d4, have: 0x10d, distance: 0xa, oneway: true},
	84:  {want: 0x3d9, have: 0x139, distance: 0xa, oneway: true},
	85:  {want: 0x3e5, have: 0x15e, distance: 0xa, oneway: true},
	86:  {want: 0x3e9, have: 0x1be, distance: 0xa, oneway: true},
	87:  {want: 0x3fa, have: 0x139, distance: 0xa, oneway: true},
	88:  {want: 0x40c, have: 0x139, distance: 0xa, oneway: true},
	89:  {want: 0x423, have: 0x139, distance: 0xa, oneway: true},
	90:  {want: 0x429, have: 0x139, distance: 0xa, oneway: true},
	91:  {want: 0x431, have: 0x139, distance: 0xa, oneway: true},
	92:  {want: 0x43b, have: 0x139, distance: 0xa, oneway: true},
	93:  {want: 0x43e, have: 0x1e1, distance: 0xa, oneway: true},
	94:  {want: 0x445, have: 0x139, distance: 0xa, oneway: true},
	95:  {want: 0x450, have: 0x139, distance: 0xa, oneway: true},
	96:  {want: 0x461, have: 0x139, distance: 0xa, oneway: true},
	97:  {want: 0x467, have: 0x3e2, distance: 0xa, oneway: true},
	98:  {want: 0x46f, have: 0x139, distance: 0xa, oneway: true},
	99:  {want: 0x476, have: 0x3e2, distance: 0xa, oneway: true},
	100: {want: 0x3883, have: 0x139, distance: 0xa, oneway: true},
	101: {want: 0x480, have: 0x139, distance: 0xa, oneway: true},
	102: {want: 0x482, have: 0x139, distance: 0xa, oneway: true},
	103: {want: 0x494, have: 0x3e2, distance: 0xa, oneway: true},
	104: {want: 0x49d, have: 0x139, distance: 0xa, oneway: true},
	105: {want: 0x4ac, have: 0x529, distance: 0xa, oneway: true},
	106: {want: 0x4b4, have: 0x139, distance: 0xa, oneway: true},
	107: {want: 0x4bc, have: 0x3e2, distance: 0xa, oneway: true},
	108: {want: 0x4e5, have: 0x15e, distance: 0xa, oneway: true},
	109: {want: 0x4f2, have: 0x139, distance: 0xa, oneway: true},
	110: {want: 0x512, have: 0x139, distance: 0xa, oneway: true},
	111: {want: 0x518, have: 0x139, distance: 0xa, oneway: true},
	112: {want: 0x52f, have: 0x139, distance: 0xa, oneway: true},
} // Size: 702 bytes

// matchScript holds pairs of scriptIDs where readers of one script
// can typically also read the other. Each is associated with a confidence.
var matchScript = []scriptIntelligibility{ // 26 elements
	0:  {wantLang: 0x432, haveLang: 0x432, wantScript: 0x5a, haveScript: 0x20, distance: 0x5},
	1:  {wantLang: 0x432, haveLang: 0x432, wantScript: 0x20, haveScript: 0x5a, distance: 0x5},
	2:  {wantLang: 0x58, haveLang: 0x3e2, wantScript: 0x5a, haveScript: 0x20, distance: 0xa},
	3:  {wantLang: 0xa5, haveLang: 0x139, wantScript: 0xe, haveScript: 0x5a, distance: 0xa},
	4:  {wantLang: 0x1d7, haveLang: 0x3e2, wantScript: 0x8, haveScript: 0x20, distance: 0xa},
	5:  {wantLang: 0x210, haveLang: 0x139, wantScript: 0x2e, haveScript: 0x5a, distance: 0xa},
	6:  {wantLang: 0x24a, haveLang: 0x139, wantScript: 0x4e, haveScript: 0x5a, distance: 0xa},
	7:  {wantLang: 0x251, haveLang: 0x139, wantScript: 0x52, haveScript: 0x5a, distance: 0xa},
	8:  {wantLang: 0x2b8, haveLang: 0x139, wantScript: 0x57, haveScript: 0x5a, distance: 0xa},
	9:  {wantLang: 0x304, haveLang: 0x139, wantScript: 0x6e, haveScript: 0x5a, distance: 0xa},
	10: {wantLang: 0x331, haveLang: 0x139, wantScript: 0x75, haveScript: 0x5a, distance: 0xa},
	11: {wantLang: 0x351, haveLang: 0x139, wantScript: 0x22, haveScript: 0x5a, distance: 0xa},
	12: {wantLang: 0x395, haveLang: 0x139, wantScript: 0x81, haveScript: 0x5a, distance: 0xa},
	13: {wantLang: 0x39d, haveLang: 0x139, wantScript: 0x36, haveScript: 0x5a, distance: 0xa},
	14: {wantLang: 0x3be, haveLang: 0x139, wantScript: 0x5, haveScript: 0x5a, distance: 0xa},
	15: {wantLang: 0x3fa, haveLang: 0x139, wantScript: 0x5, haveScript: 0x5a, distance: 0xa},
	16: {wantLang: 0x40c, haveLang: 0x139, wantScript: 0xcf, haveScript: 0x5a, distance: 0xa},
	17: {wantLang: 0x450, haveLang: 0x139, wantScript: 0xde, haveScript: 0x5a, distance: 0xa},
	18: {wantLang: 0x461, haveLang: 0x139, wantScript: 0xe1, haveScript: 0x5a, distance: 0xa},
	19: {wantLang: 0x46f, haveLang: 0x139, wantScript: 0x2c, haveScript: 0x5a, distance: 0xa},
	20: {wantLang: 0x476, haveLang: 0x3e2, wantScript: 0x5a, haveScript: 0x20, distance: 0xa},
	21: {wantLang: 0x4b4, haveLang: 0x139, wantScript: 0x5, haveScript: 0x5a, distance: 0xa},
	22: {wantLang: 0x4bc, haveLang: 0x3e2, wantScript: 0x5a, haveScript: 0x20, distance: 0xa},
	23: {wantLang: 0x512, haveLang: 0x139, wantScript: 0x3e, haveScript: 0x5a, distance: 0xa},
	24: {wantLang: 0x529, haveLang: 0x529, wantScript: 0x3b, haveScript: 0x3c, distance: 0xf},
	25: {wantLang: 0x529, haveLang: 0x529, wantScript: 0x3c, haveScript: 0x3b, distance: 0x13},
} // Size: 232 bytes

var matchRegion = []regionIntelligibility{ // 15 elements
	0:  {lang: 0x3a, script: 0x0, group: 0x4, distance: 0x4},
	1:  {lang: 0x3a, script: 0x0, group: 0x84, distance: 0x4},
	2:  {lang: 0x139, script: 0x0, group: 0x1, distance: 0x4},
	3:  {lang: 0x139, script: 0x0, group: 0x81, distance: 0x4},
	4:  {lang: 0x13e, script: 0x0, group: 0x3, distance: 0x4},
	5:  {lang: 0x13e, script: 0x0, group: 0x83, distance: 0x4},
	6:  {lang: 0x3c0, script: 0x0, group: 0x3, distance: 0x4},
	7:  {lang: 0x3c0, script: 0x0, group: 0x83, distance: 0x4},
	8:  {lang: 0x529, script: 0x3c, group: 0x2, distance: 0x4},
	9:  {lang: 0x529, script: 0x3c, group: 0x82, distance: 0x4},
	10: {lang: 0x3a, script: 0x0, group: 0x80, distance: 0x5},
	11: {lang: 0x139, script: 0x0, group: 0x80, distance: 0x5},
	12: {lang: 0x13e, script: 0x0, group: 0x80, distance: 0x5},
	13: {lang: 0x3c0, script: 0x0, group: 0x80, distance: 0x5},
	14: {lang: 0x529, script: 0x3c, group: 0x80, distance: 0x5},
} // Size: 114 bytes

// Total table size 1472 bytes (1KiB); checksum: F86C669
