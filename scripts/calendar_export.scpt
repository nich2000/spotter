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
	set totalSeconds to time of value
	set hourValue to totalSeconds div 3600
	set minuteValue to (totalSeconds mod 3600) div 60
	set secondValue to totalSeconds mod 60
	set timezoneValue to "+03:00"
	return (year of value as text) & "-" & my twoDigits(month of value as integer) & "-" & my twoDigits(day of value) & "T" & my twoDigits(hourValue) & ":" & my twoDigits(minuteValue) & ":" & my twoDigits(secondValue) & timezoneValue
end isoDate

on twoDigits(value)
	set numberValue to value as integer
	if numberValue < 10 then return "0" & numberValue
	return numberValue as text
end twoDigits

on readyCalendars()
	-- LaunchServices can start Calendar when its AppleEvent launch command fails (-600).
	try
		do shell script "/usr/bin/open -g -b com.apple.iCal"
	on error errorMessage number errorNumber
		error "Calendar startup failed (LaunchServices): " & errorMessage number errorNumber
	end try

	repeat with attempt from 1 to 20
		try
			tell application id "com.apple.iCal" to return every calendar
		on error errorMessage number errorNumber
			-- Retry only the transient 'application is not running' error.
			if errorNumber is not -600 then error "Calendar readiness failed: " & errorMessage number errorNumber
			if attempt is 20 then error "Calendar did not become ready after 20 attempts: " & errorMessage number errorNumber
			delay 0.5
		end try
	end repeat
end readyCalendars

on run argv
	set todayStart to date (item 1 of argv)
	set rangeEnd to date (item 2 of argv)
	set rows to {}
	set availableCalendars to my readyCalendars()
	
	tell application id "com.apple.iCal"
		repeat with cal in availableCalendars
			set calName to name of cal
			if calName is "Scheduled Reminders" then
				set skipCalendar to true
			else
				set skipCalendar to false
			end if
			if skipCalendar then
				-- Reminders due dates are rendered by the Reminders source.
			else
			repeat with ev in (every event of cal whose end date > todayStart and start date < rangeEnd)
				set eventTitle to summary of ev
				set eventStart to (start date of ev) as date
				set eventEnd to (end date of ev) as date
				set eventLocation to ""
				set eventNotes to ""
				try
					set eventLocation to location of ev
				end try
				try
					set eventNotes to description of ev
				end try
				set row to "{\"title\":\"" & my jsonEscape(eventTitle) & "\",\"start\":\"" & my isoDate(eventStart) & "\",\"end\":\"" & my isoDate(eventEnd) & "\",\"location\":\"" & my jsonEscape(eventLocation) & "\",\"calendar\":\"" & my jsonEscape(calName) & "\",\"notes\":\"" & my jsonEscape(eventNotes) & "\"}"
				set end of rows to row
			end repeat
			end if
		end repeat
	end tell
	
	set AppleScript's text item delimiters to ","
	set output to "[" & (rows as text) & "]"
	set AppleScript's text item delimiters to ""
	return output
end run
