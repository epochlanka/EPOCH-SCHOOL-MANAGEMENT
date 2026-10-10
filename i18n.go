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
	"pickup.new.title": {"Pickup change: %s", "ගෙන යාමේ වෙනසක්: %s", "அழைத்துச் செல்லும் மாற்றம்: %s"},
	"pickup.new.body":  {"%s (%s) on %s: %s", "%s (%s) – %s: %s", "%s (%s) – %s: %s"},
	"pickup.kind.collector": {
		"%s (%s, %s) will collect the child",
		"%s (%s, %s) දරුවා රැගෙන යනු ඇත",
		"%s (%s, %s) பிள்ளையை அழைத்துச் செல்வார்",
	},
	"pickup.kind.no_bus": {
		"Not taking the school bus; a parent will collect",
		"පාසල් බසයේ නොයයි; දෙමාපියෙකු රැගෙන යයි",
		"பள்ளிப் பேருந்தில் செல்லமாட்டார்; பெற்றோர் அழைத்துச் செல்வார்",
	},
	"pickup.kind.early": {"Early pickup at %s", "%s ට කලින් රැගෙන යාම", "%s மணிக்கு முன்கூட்டியே அழைத்துச் செல்லுதல்"},
	"pickup.ack.title":  {"Pickup change confirmed", "ගෙන යාමේ වෙනස තහවුරු කළා", "அழைத்துச் செல்லும் மாற்றம் உறுதிப்படுத்தப்பட்டது"},
	"pickup.ack.body": {
		"The school has noted the pickup change for %s on %s.%s",
		"%s සඳහා %s දින ගෙන යාමේ වෙනස පාසල සටහන් කර ගත්තා.%s",
		"%[2]s அன்று %[1]s க்கான அழைத்துச் செல்லும் மாற்றத்தைப் பள்ளி பதிவு செய்துள்ளது.%[3]s",
	},
	"pickup.declined.title": {"Pickup change not accepted", "ගෙන යාමේ වෙනස පිළිගත නොහැක", "அழைத்துச் செல்லும் மாற்றம் ஏற்கப்படவில்லை"},
	"pickup.declined.body": {
		"The school could not accept the pickup change for %s on %s. Please call the school office.%s",
		"%s සඳහා %s දින ගෙන යාමේ වෙනස පාසලට පිළිගත නොහැකි විය. කරුණාකර පාසල් කාර්යාලය අමතන්න.%s",
		"%[2]s அன்று %[1]s க்கான அழைத்துச் செல்லும் மாற்றத்தை ஏற்க முடியவில்லை. பள்ளி அலுவலகத்தைத் தொடர்பு கொள்ளவும்.%[3]s",
	},
	"prog.att":       {"Attendance over the last 30 days is %s%%%s.", "පසුගිය දින 30 තුළ පැමිණීම %s%%%s.", "கடந்த 30 நாட்களில் வருகை %s%%%s."},
	"prog.att.up":    {" (up from %s%%)", " (%s%% සිට ඉහළ ගොස් ඇත)", " (%s%% இலிருந்து உயர்வு)"},
	"prog.att.down":  {" (down from %s%%)", " (%s%% සිට පහළ වැටී ඇත)", " (%s%% இலிருந்து குறைவு)"},
	"prog.strong":    {"Doing well in %s.", "%s හි හොඳින් කටයුතු කරයි.", "%s இல் சிறப்பாகச் செயல்படுகிறார்."},
	"prog.focus":     {"Needs more support in %s.", "%s සඳහා වැඩි සහායක් අවශ්‍යයි.", "%s இல் கூடுதல் உதவி தேவை."},
	"prog.noresults": {"No exam results have been published yet.", "තවම විභාග ප්‍රතිඵල ප්‍රකාශ කර නැත.", "இன்னும் தேர்வு முடிவுகள் வெளியிடப்படவில்லை."},
	"prog.hw":        {"%s homework task(s) are due soon.", "ගෙදර වැඩ %s ක් ළඟදීම භාර දිය යුතුය.", "%s வீட்டுப்பாடங்கள் விரைவில் சமர்ப்பிக்க வேண்டும்."},
	"step.att": {
		"Aim for full attendance — regular attendance makes the biggest difference to results.",
		"සම්පූර්ණ පැමිණීම ඉලක්ක කරන්න — නිතිපතා පැමිණීම ප්‍රතිඵලවලට විශාලතම වෙනස ඇති කරයි.",
		"முழு வருகையை இலக்காகக் கொள்ளுங்கள் — தொடர்ச்சியான வருகை முடிவுகளில் பெரிய மாற்றத்தை ஏற்படுத்தும்.",
	},
	"step.focus": {
		"Spend 20 minutes a day revising %s, and ask the teacher which topics to practise.",
		"දිනකට මිනිත්තු 20ක් %s පුනරීක්ෂණය කරන්න; පුහුණු විය යුතු මාතෘකා ගුරුවරයාගෙන් අසන්න.",
		"தினமும் 20 நிமிடங்கள் %s ஐ மீளாய்வு செய்யுங்கள்; எந்தப் பகுதிகளைப் பயிற்சி செய்ய வேண்டும் என ஆசிரியரிடம் கேளுங்கள்.",
	},
	"step.hw": {
		"Check the homework diary together each evening — %s task(s) are coming up.",
		"සෑම සවසකම එක්ව ගෙදර වැඩ දිනපොත පරීක්ෂා කරන්න — කාර්යයන් %s ක් ඉදිරියේ ඇත.",
		"ஒவ்வொரு மாலையும் வீட்டுப்பாட நாட்குறிப்பை ஒன்றாகப் பாருங்கள் — %s பணிகள் வரவுள்ளன.",
	},
	"step.meet": {
		"Book a short meeting with the class teacher to agree on a plan.",
		"සැලැස්මක් එකඟ කර ගැනීමට පන්ති භාර ගුරුවරයා සමඟ කෙටි හමුවක් වෙන් කරන්න.",
		"ஒரு திட்டத்தை ஒப்புக்கொள்ள வகுப்பாசிரியருடன் சிறிய சந்திப்பை ஏற்பாடு செய்யுங்கள்.",
	},
	"step.praise": {
		"Praise the effort in %s — it is clearly paying off.",
		"%s හි දක්වන උත්සාහය අගය කරන්න — එය පැහැදිලිවම ඵල දරයි.",
		"%s இல் காட்டும் முயற்சியைப் பாராட்டுங்கள் — அது தெளிவாகப் பலன் தருகிறது.",
	},
	"step.keep": {
		"Keep up the steady routine — everything is on track.",
		"ස්ථාවර දින චර්යාව දිගටම පවත්වා ගන්න — සියල්ල නිවැරදි මාර්ගයේ ඇත.",
		"நிலையான நடைமுறையைத் தொடருங்கள் — அனைத்தும் சரியான பாதையில் உள்ளது.",
	},
	"early.title": {"Early leave recorded: %s", "කලින් පිටවීම සටහන් විය: %s", "முன்கூட்டிய வெளியேற்றம் பதிவு: %s"},
	"early.body": {
		"%s left school at %s on %s with %s (%s). Reason: %s – %s",
		"%[1]s %[3]s දින %[2]s ට %[4]s (%[5]s) සමඟ පාසලෙන් පිටව ගියේය. හේතුව: %[6]s – %[7]s",
		"%[1]s %[3]s அன்று %[2]s மணிக்கு %[4]s (%[5]s) உடன் பள்ளியை விட்டுச் சென்றார். காரணம்: %[6]s – %[7]s",
	},
	"early.bereavement":    {"Family bereavement / funeral", "පවුලේ මරණයක් / අවමංගල්‍යය", "குடும்ப மரணம் / இறுதிச் சடங்கு"},
	"early.illness":        {"Student unwell", "සිසුවා අසනීපයි", "மாணவர் உடல்நலக்குறைவு"},
	"early.family_illness": {"Family illness / hospital", "පවුලේ අසනීපයක් / රෝහල", "குடும்ப நோய் / மருத்துவமனை"},
	"early.medical":        {"Medical / dental appointment", "වෛද්‍ය / දන්ත හමුවීම", "மருத்துவ / பல் மருத்துவ சந்திப்பு"},
	"early.emergency":      {"Family emergency", "පවුලේ හදිසි අවස්ථාවක්", "குடும்ப அவசரநிலை"},
	"early.religious":      {"Religious / cultural event", "ආගමික / සංස්කෘතික උත්සවය", "சமய / கலாச்சார நிகழ்வு"},
	"early.other":          {"Other", "වෙනත්", "மற்றவை"},
	"rel.mother":           {"mother", "මව", "தாய்"},
	"rel.father":           {"father", "පියා", "தந்தை"},
	"rel.grandparent":      {"grandparent", "සීයා / ආච්චි", "தாத்தா / பாட்டி"},
	"rel.guardian":         {"guardian", "භාරකරු", "பாதுகாவலர்"},
	"rel.sibling":          {"brother / sister", "සහෝදරයා / සහෝදරිය", "சகோதரர் / சகோதரி"},
	"rel.relative":         {"relative", "ඥාතියෙක්", "உறவினர்"},
	"rel.driver":           {"family driver", "පවුලේ රියදුරු", "குடும்ப ஓட்டுநர்"},
	"rel.other":            {"other", "වෙනත්", "மற்றவர்"},
	"library.title":        {"Library book overdue", "පුස්තකාල පොත කල් ඉකුත්", "நூலகப் புத்தகம் தாமதம்"},
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
