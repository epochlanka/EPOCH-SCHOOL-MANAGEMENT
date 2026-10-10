package main

import (
	"fmt"
	"strings"
	"time"
)

// Languages a user can choose (stored in users.language).
var languages = []string{"English", "Sinhala", "Tamil"}

var langIdx = map[string]int{"English": 0, "Sinhala": 1, "Tamil": 2}

func validLanguage(l string) bool { _, ok := langIdx[l]; return ok }

// msgCatalog holds every automatic message in English, Sinhala and Tamil.
// Placeholders use Go fmt verbs; %[n]s picks argument n so translations can
// reorder them.
var msgCatalog = map[string][3]string{
	"absent.title": {"Absent Alert", "නොපැමිණීමේ දැනුම්දීම", "வருகையின்மை அறிவிப்பு"},
	"absent.body": {
		"Dear Parent, your child %s was marked ABSENT today (%s). Please contact the class teacher for more information.",
		"හිතවත් දෙමාපියනි, ඔබේ දරුවා %s අද (%s) පාසලට නොපැමිණි බව සටහන් වී ඇත. වැඩි විස්තර සඳහා කරුණාකර පන්ති භාර ගුරුවරයා අමතන්න.",
		"அன்புள்ள பெற்றோரே, உங்கள் பிள்ளை %s இன்று (%s) பள்ளிக்கு வரவில்லை எனப் பதிவு செய்யப்பட்டுள்ளது. மேலதிக விவரங்களுக்கு வகுப்பாசிரியரைத் தொடர்பு கொள்ளவும்.",
	},
	"late.title": {"Late Arrival", "ප්‍රමාද වී පැමිණීම", "தாமதமான வருகை"},
	"late.body": {
		"Dear Parent, your child %s arrived LATE to school today (%s).",
		"හිතවත් දෙමාපියනි, ඔබේ දරුවා %s අද (%s) පාසලට ප්‍රමාද වී පැමිණියේය.",
		"அன்புள்ள பெற்றோரே, உங்கள் பிள்ளை %s இன்று (%s) பள்ளிக்குத் தாமதமாக வந்தார்.",
	},
	"present.title": {"Attendance Update", "පැමිණීමේ යාවත්කාලීනය", "வருகைத் தகவல்"},
	"present.body": {
		"Dear Parent, your child %s was marked Present today (%s). Have a great day! – %s",
		"හිතවත් දෙමාපියනි, ඔබේ දරුවා %s අද (%s) පාසලට පැමිණ ඇත. සුබ දවසක්! – %s",
		"அன்புள்ள பெற்றோரே, உங்கள் பிள்ளை %s இன்று (%s) பள்ளிக்கு வருகை தந்துள்ளார். இனிய நாளாக அமையட்டும்! – %s",
	},
	"feenew.title": {"New Fee Issued", "නව ගාස්තුවක් නිකුත් කෙරිණි", "புதிய கட்டணம் வழங்கப்பட்டது"},
	"feenew.body": {
		"Dear Parent, a new fee \"%s\" of %s has been issued. Due date: %s.",
		"හිතවත් දෙමාපියනි, \"%s\" සඳහා %s ක නව ගාස්තුවක් නිකුත් කර ඇත. ගෙවිය යුතු දිනය: %s.",
		"அன்புள்ள பெற்றோரே, \"%s\" க்கான %s புதிய கட்டணம் வழங்கப்பட்டுள்ளது. செலுத்த வேண்டிய தேதி: %s.",
	},
	"paid.title": {"Payment Received", "ගෙවීම ලැබිණි", "கட்டணம் பெறப்பட்டது"},
	"paid.body": {
		"Dear Parent, we have received %s for \"%s\" (%s). Thank you!",
		"හිතවත් දෙමාපියනි, \"%[2]s\" (%[3]s) සඳහා %[1]s ලැබී ඇත. ස්තූතියි!",
		"அன்புள்ள பெற்றோரே, \"%[2]s\" (%[3]s) க்கான %[1]s பெறப்பட்டது. நன்றி!",
	},
	"feedue.title": {"Fee Reminder", "ගාස්තු සිහිකැඳවීම", "கட்டண நினைவூட்டல்"},
	"feedue.body": {
		"Dear Parent, fee \"%s\" of %s for %s is due on %s. Please ignore if already paid.",
		"හිතවත් දෙමාපියනි, %[3]s සඳහා \"%[1]s\" ගාස්තුව වන %[2]s %[4]s දිනට ගෙවිය යුතුය. දැනටමත් ගෙවා ඇත්නම් කරුණාකර නොසලකා හරින්න.",
		"அன்புள்ள பெற்றோரே, %[3]s இற்கான \"%[1]s\" கட்டணம் %[2]s, %[4]s அன்று செலுத்தப்பட வேண்டும். ஏற்கனவே செலுத்தியிருந்தால் இதைப் புறக்கணிக்கவும்.",
	},
	"feeover.title": {"Fee Overdue", "ගාස්තුව කල් ඉකුත් වී ඇත", "கட்டணம் காலாவதியானது"},
	"feeover.body": {
		"Dear Parent, the fee \"%s\" of %s for %s was due on %s and is now overdue. Kindly settle it at the earliest. Please ignore if already paid.",
		"හිතවත් දෙමාපියනි, %[3]s සඳහා \"%[1]s\" ගාස්තුව වන %[2]s %[4]s දිනට ගෙවිය යුතු වූ අතර දැන් කල් ඉකුත් වී ඇත. කරුණාකර හැකි ඉක්මනින් ගෙවන්න. දැනටමත් ගෙවා ඇත්නම් නොසලකා හරින්න.",
		"அன்புள்ள பெற்றோரே, %[3]s இற்கான \"%[1]s\" கட்டணம் %[2]s, %[4]s அன்று செலுத்தப்பட வேண்டியது, தற்போது காலாவதியாகியுள்ளது. விரைவில் செலுத்தவும். ஏற்கனவே செலுத்தியிருந்தால் இதைப் புறக்கணிக்கவும்.",
	},
	"hw.title":     {"Homework Assigned", "ගෙදර වැඩ පවරා ඇත", "வீட்டுப்பாடம் வழங்கப்பட்டது"},
	"hw.body":      {"%s (%s): %s. Due: %s.", "%s (%s): %s. භාර දිය යුතු දිනය: %s.", "%s (%s): %s. சமர்ப்பிக்க வேண்டிய தேதி: %s."},
	"exam.title":   {"Exam Notice", "විභාග දැන්වීම", "தேர்வு அறிவிப்பு"},
	"exam.body":    {"%s – %s for %s. Date: %s", "%[3]s සඳහා %[2]s – %[1]s. දිනය: %[4]s", "%[3]s இற்கான %[2]s – %[1]s. தேதி: %[4]s"},
	"exam.time":    {", Time: %s", ", වේලාව: %s", ", நேரம்: %s"},
	"result.title": {"Result Published", "ප්‍රතිඵල ප්‍රකාශයට පත් විය", "முடிவு வெளியிடப்பட்டது"},
	"result.body": {
		"%s – %s %s: %s/%s (%s%%, Grade %s).",
		"%s – %s %s: %s/%s (%s%%, සාමාර්ථය %s).",
		"%s – %s %s: %s/%s (%s%%, தரம் %s).",
	},
	"bus.title":      {"Bus Update", "බස් යාවත්කාලීනය", "பேருந்து தகவல்"},
	"bus.titlebus":   {"Bus Update – Bus %s", "බස් යාවත්කාලීනය – බස් %s", "பேருந்து தகவல் – பேருந்து %s"},
	"message.title":  {"New message from %s", "%s ගෙන් නව පණිවිඩයක්", "%s இடமிருந்து புதிய செய்தி"},
	"leavereq.title": {"Leave request: %s", "නිවාඩු ඉල්ලීම: %s", "விடுப்புக் கோரிக்கை: %s"},
	"leavereq.body": {
		"%s requested %s for %s. %s",
		"%[1]s, %[3]s සඳහා %[2]s ඉල්ලා ඇත. %[4]s",
		"%[1]s %[3]s க்கு %[2]s கோரியுள்ளார். %[4]s",
	},
	"leave.approved.title": {"Leave approved", "නිවාඩු අනුමත විය", "விடுப்பு அங்கீகரிக்கப்பட்டது"},
	"leave.rejected.title": {"Leave rejected", "නිවාඩු ප්‍රතික්ෂේප විය", "விடுப்பு நிராகரிக்கப்பட்டது"},
	"leave.approved.body": {
		"Your %s for %s was approved by %s.",
		"%[2]s සඳහා ඔබේ %[1]s %[3]s විසින් අනුමත කරන ලදී.",
		"%[2]s க்கான உங்கள் %[1]s %[3]s அவர்களால் அங்கீகரிக்கப்பட்டது.",
	},
	"leave.rejected.body": {
		"Your %s for %s was rejected by %s.",
		"%[2]s සඳහා ඔබේ %[1]s %[3]s විසින් ප්‍රතික්ෂේප කරන ලදී.",
		"%[2]s க்கான உங்கள் %[1]s %[3]s அவர்களால் நிராகரிக்கப்பட்டது.",
	},
	"leave.note":      {" Note: %s", " සටහන: %s", " குறிப்பு: %s"},
	"leave.casual":    {"casual leave", "අනියම් නිවාඩු", "சாதாரண விடுப்பு"},
	"leave.medical":   {"medical leave", "වෛද්‍ය නිවාඩු", "மருத்துவ விடுப்பு"},
	"leave.duty":      {"duty leave", "රාජකාරි නිවාඩු", "கடமை விடுப்பு"},
	"leave.half_day":  {"a half day", "අර්ධ දින නිවාඩු", "அரை நாள் விடுப்பு"},
	"leave.maternity": {"maternity leave", "මාතෘ නිවාඩු", "மகப்பேறு விடுப்பு"},
	"leave.other":     {"leave", "නිවාඩු", "விடுப்பு"},
	"relief.title":    {"Relief duty on %s", "%s ආදේශක රාජකාරිය", "%s அன்று மாற்றுப் பணி"},
	"relief.line": {
		"Period %s (%s): %s %s – covering for %s",
		"කාලච්ඡේදය %s (%s): %s %s – %s වෙනුවට",
		"பாடவேளை %s (%s): %s %s – %s க்குப் பதிலாக",
	},
	"unfilled.title": {"Relief cover needed", "ආදේශක ගුරුවරු අවශ්‍යයි", "மாற்று ஆசிரியர் தேவை"},
	"unfilled.body": {
		"%s period(s) on %s have no free teacher. Please assign cover in Substitutions.",
		"%[2]s දින කාලච්ඡේද %[1]s කට නිදහස් ගුරුවරයෙක් නැත. කරුණාකර ආදේශක ගුරුවරු පිටුවෙන් පවරන්න.",
		"%[2]s அன்று %[1]s பாடவேளைகளுக்கு இலவச ஆசிரியர் இல்லை. மாற்று ஆசிரியர்கள் பக்கத்தில் ஒதுக்கவும்.",
	},
	"welcome.title": {"Welcome to %s", "%s වෙත සාදරයෙන් පිළිගනිමු", "%s க்கு வரவேற்கிறோம்"},
	"welcome.body": {
		"Dear Parent, welcome to %s! %s has been enrolled in %s. Download the school app and sign in with %s to receive attendance, homework and fee updates.",
		"හිතවත් දෙමාපියනි, %[1]s වෙත සාදරයෙන් පිළිගනිමු! %[2]s %[3]s පන්තියට ඇතුළත් කර ඇත. පැමිණීම, ගෙදර වැඩ සහ ගාස්තු පිළිබඳ තොරතුරු ලබා ගැනීමට පාසල් යෙදුමට %[4]s සමඟ පුරනය වන්න.",
		"அன்புள்ள பெற்றோரே, %[1]s க்கு வரவேற்கிறோம்! %[2]s %[3]s வகுப்பில் சேர்க்கப்பட்டுள்ளார். வருகை, வீட்டுப்பாடம் மற்றும் கட்டணத் தகவல்களைப் பெற பள்ளிச் செயலியில் %[4]s மூலம் உள்நுழையவும்.",
	},
	"library.title": {"Library book overdue", "පුස්තකාල පොත කල් ඉකුත්", "நூலகப் புத்தகம் தாமதம்"},
	"library.body": {
		"Dear Parent, the library book \"%s\" borrowed by %s was due on %s. Please return it to the library.",
		"හිතවත් දෙමාපියනි, %[2]s ලබාගත් \"%[1]s\" පුස්තකාල පොත %[3]s දිනට ආපසු දිය යුතුව තිබුණි. කරුණාකර එය පුස්තකාලයට ආපසු දෙන්න.",
		"அன்புள்ள பெற்றோரே, %[2]s எடுத்த \"%[1]s\" நூலகப் புத்தகம் %[3]s அன்று திருப்பித் தரப்பட வேண்டியது. தயவுசெய்து நூலகத்தில் திருப்பிக் கொடுக்கவும்.",
	},
}

// L returns catalogue message key in lang, filled with args.
func L(lang, key string, args ...any) string {
	m, ok := msgCatalog[key]
	if !ok {
		return key
	}
	f := m[langIdx[lang]]
	if f == "" {
		f = m[0]
	}
	if len(args) == 0 {
		return f
	}
	out := fmt.Sprintf(f, args...)
	// A translation may use fewer values than given; drop the extras rather
	// than leaking "%!(EXTRA ...)" into a message.
	for n := len(args) - 1; n >= 0 && strings.Contains(out, "%!(EXTRA"); n-- {
		out = fmt.Sprintf(f, args[:n]...)
	}
	return out
}

var monthNames = map[string][12]string{
	"Sinhala": {"ජනවාරි", "පෙබරවාරි", "මාර්තු", "අප්‍රේල්", "මැයි", "ජූනි", "ජූලි", "අගෝස්තු", "සැප්තැම්බර්", "ඔක්තෝබර්", "නොවැම්බර්", "දෙසැම්බර්"},
	"Tamil":   {"ஜனவரி", "பிப்ரவரி", "மார்ச்", "ஏப்ரல்", "மே", "ஜூன்", "ஜூலை", "ஆகஸ்ட்", "செப்டம்பர்", "அக்டோபர்", "நவம்பர்", "டிசம்பர்"},
}

// dateL formats 2026-10-22 as "22 Oct 2026" / "22 ඔක්තෝබර් 2026" / "22 அக்டோபர் 2026".
func dateL(d, lang string) string {
	names, ok := monthNames[lang]
	if !ok {
		return prettyDate(d)
	}
	t, err := time.Parse(dateLayout, d)
	if err != nil {
		return d
	}
	return fmt.Sprintf("%d %s %d", t.Day(), names[t.Month()-1], t.Year())
}

// dateRangeL formats a from–to range of dates.
func dateRangeL(from, to, lang string) string {
	if to == "" || to == from {
		return dateL(from, lang)
	}
	return dateL(from, lang) + " – " + dateL(to, lang)
}

// NotifyEach sends a message built separately for each recipient language, so
// every user gets it in the language they chose. It returns the recipients reached.
func (n *Notifier) NotifyEach(category string, userIDs []int64, channels []string, build func(lang string) (title, body string)) (int, error) {
	var ids []int64
	for _, id := range userIDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	rows, err := n.a.db.Query(`SELECT id, language FROM users WHERE id IN (`+placeholders(len(ids))+`)`, int64Args(ids)...)
	if err != nil {
		return 0, err
	}
	groups := map[string][]int64{}
	for rows.Next() {
		var id int64
		var lang string
		if rows.Scan(&id, &lang) == nil {
			if !validLanguage(lang) {
				lang = "English"
			}
			groups[lang] = append(groups[lang], id)
		}
	}
	rows.Close()
	total := 0
	for lang, group := range groups {
		title, body := build(lang)
		c, err := n.Notify(category, title, body, group, channels)
		if err != nil {
			return total, err
		}
		total += c
	}
	return total, nil
}

// userLanguage returns a user's chosen language (English if unset).
func (a *App) userLanguage(id int64) string {
	var lang string
	a.db.QueryRow(`SELECT language FROM users WHERE id=?`, id).Scan(&lang)
	if !validLanguage(lang) {
		return "English"
	}
	return lang
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}
