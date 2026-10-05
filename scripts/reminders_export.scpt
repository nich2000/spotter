use framework "Foundation"
use scripting additions

on jsonEscape(value)
	set textValue to value as text
	set textValue to my replaceText(textValue, "\\", "\\\\")
	set textValue to my replaceText(textValue, "\"", "\\\"")
	set textValue to my replaceText(textValue, return, "\\n")
	set textValue to my replaceText(textValue, linefeed, "\\n")
	set textValue to my replaceText(textValue, tab, "\\t")
	return textValue
end jsonEscape

on replaceText(value, findText, replacementText)
	set AppleScript's text item delimiters to findText
	set parts to text items of value
	set AppleScript's text item delimiters to replacementText
	set joined to parts as text
	set AppleScript's text item delimiters to ""
	return joined
end replaceText

on isoDate(value)
	set formatter to current application's NSDateFormatter's alloc()'s init()
	formatter's setLocale:(current application's NSLocale's localeWithLocaleIdentifier:"en_US_POSIX")
	formatter's setDateFormat:"yyyy-MM-dd'T'HH:mm:ssXXX"
	formatter's setTimeZone:(current application's NSTimeZone's localTimeZone())
	return (formatter's stringFromDate:value) as text
end isoDate

on twoDigits(value)
	set numberValue to value as integer
	if numberValue < 10 then return "0" & numberValue
	return numberValue as text
end twoDigits

set rows to {}

tell application "Reminders"
	repeat with reminderList in lists
		set listName to name of reminderList
		set openNames to name of (reminders of reminderList whose completed is false)
		set openDates to due date of (reminders of reminderList whose completed is false)
		repeat with i from 1 to count of openNames
			set itemTitle to item i of openNames
			set itemDueDate to item i of openDates
			set dueJSON to "null"
			if itemDueDate is not missing value then set dueJSON to "\"" & my isoDate(itemDueDate) & "\""
			set row to "{\"title\":\"" & my jsonEscape(itemTitle) & "\",\"dueDate\":" & dueJSON & ",\"list\":\"" & my jsonEscape(listName) & "\",\"priority\":0,\"notes\":\"\"}"
				set end of rows to row
			end repeat
	end repeat
end tell

set AppleScript's text item delimiters to ","
set output to "[" & (rows as text) & "]"
set AppleScript's text item delimiters to ""
return output
