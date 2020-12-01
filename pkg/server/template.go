package server

const Template = `
<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport"
          content="width=device-width, user-scalable=no, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0">
    <meta http-equiv="X-UA-Compatible" content="ie=edge">
    <title>CronHPA Job Monitor</title>
</head>
<body>
	<center style="padding: 24px 0 24px 0">Cron Engine Job Monitor</center>
	<table class="gridtable">
      <tr>
		<th>CronHPA</th>
		<th>Namespace</th>	
		<th>Id</th>
		<th>Job</th>
		<th>Pre</th>
		<th>Next</th> 
      </tr>
{{range .Items}}
	  <tr>
	    <td>{{ .CronHPA }}</td>
		<td>{{ .Namespace }}</td>
		<td>{{ .Id }}</td>
		<td>{{ .Name }}</td>
		<td>{{ .Pre }}</td>
		<td>{{ .Next }}</td>
      </tr>
{{end}}
 	</table>
<style>
table.gridtable {margin:0 auto; font-family: verdana,arial,sans-serif;font-size:12px;color:#333333;border-width: 1px;border-color: #666666;border-collapse: collapse;}
table.gridtable th {border-width: 1px;padding: 8px;border-style: solid;border-color: #666666;background-color: #dedede;}
table.gridtable td {border-width: 1px;padding: 8px;border-style: solid;border-color: #666666;background-color: #ffffff;}
</style>
</body>
`
