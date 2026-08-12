package fanqie

// Glyph order originally documented by the fanqienovel-decryptor project.
// Question marks represent unknown positions and remain untouched.
const glyphCharset = "D在主特家军然表场4要只v和?6别还g现儿岁??此象月3出战工相o男直失世F都平文什VO将真T那当?" +
	"会立些u是十张学气大爱两命全后东性通被1它乐接而感车山公了常以何可话先pi叫轻M士w着变尔快" +
	"l个说少色里安花远7难师放t报认面道S?克地度I好机U民写把万同水新没书电吃像斯5为y白几日教" +
	"看但第加候作上拉住有法r事应位利你声身国问马女他Y比父xAHNsX边美对所金活回意到z从j知又内" +
	"因点Q三定8Rb正或夫向德听更?得告并本q过记L让打f人就者去原满体做经K走如孩cG给使物?最笑部?" +
	"员等受k行一条果动光门头见往自解成处天能于名其发总母的死手入路进心来h时力多开已许d至由很" +
	"界n小与Z想代么分生口再妈望次西风种带J?实情才这?E我神格长觉间年眼无不亲关结0友信下却重己" +
	"老2音字m呢明之前高PB目太e9起稜她也W用方子英每理便四数期中C外样a海们任"

const baseGlyphID = 58344

var fallbackGlyphs = func() map[rune]rune {
	mapping := make(map[rune]rune)
	for index, character := range []rune(glyphCharset) {
		if character != '?' {
			mapping[rune(baseGlyphID+index)] = character
		}
	}
	return mapping
}()

func decrypt(text string) string {
	return stringsMap(func(character rune) rune {
		if decoded, exists := fallbackGlyphs[character]; exists {
			return decoded
		}
		return character
	}, text)
}

// Kept behind a tiny wrapper so tests can exercise decoding without exporting
// the upstream-specific map.
var stringsMap = func(mapping func(rune) rune, text string) string {
	result := make([]rune, 0, len([]rune(text)))
	for _, character := range text {
		result = append(result, mapping(character))
	}
	return string(result)
}
