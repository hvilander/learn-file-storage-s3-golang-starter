some notes from aws setup


created a bucket via the console
following these instructions: 
Uncheck "Block all public access" when creating the bucket.
Leave bucket versioning off.
Leave default encryption on with managed keys.
Leave object lock disabled   


added this json bucket polciy in the permisssions tab: 
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": "*",
            "Action": "s3:GetObject",
            "Resource": "arn:aws:s3:::tubely-192470/*"
        }
    ]
}

